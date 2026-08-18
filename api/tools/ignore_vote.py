"""Community voting tool for adding normal IRC users to the ignore list."""

import hashlib
import re
import sqlite3
import time
from pathlib import Path
from typing import Any, Callable, Dict

from .base import Tool


IGNORED_LEVEL = 0
ADMIN_LEVEL = 2
DEFAULT_THRESHOLD = 3
DEFAULT_WINDOW_SECONDS = 24 * 60 * 60

# Rizon authenticates this owner dynamically with NickServ, so the owner may
# legitimately have no elevated row in the local users table yet.
RESERVED_OWNER_NICKS = frozenset({"mathisen"})


class IgnoreVoteTool(Tool):
    """Record distinct, time-bounded votes to ignore a normal user."""

    def __init__(
        self,
        db_path: str = "data/bot.db",
        threshold: int = DEFAULT_THRESHOLD,
        window_seconds: int = DEFAULT_WINDOW_SECONDS,
        now: Callable[[], float] = time.time,
    ) -> None:
        self.db_path = Path(db_path)
        self.threshold = threshold
        self.window_seconds = window_seconds
        self._now = now
        self._ensure_schema()

    @property
    def name(self) -> str:
        return "vote_to_ignore"

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.db_path, timeout=5)
        connection.execute("PRAGMA busy_timeout = 5000")
        return connection

    def _ensure_schema(self) -> None:
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        with self._connect() as connection:
            connection.execute(
                """
                CREATE TABLE IF NOT EXISTS ignore_votes (
                    target_nick TEXT NOT NULL,
                    voter_key TEXT NOT NULL,
                    voted_at INTEGER NOT NULL,
                    PRIMARY KEY (target_nick, voter_key)
                )
                """
            )
            connection.execute(
                """
                CREATE INDEX IF NOT EXISTS idx_ignore_votes_voted_at
                ON ignore_votes(voted_at)
                """
            )

    def get_definition(self) -> Dict[str, Any]:
        return {
            "type": "function",
            "name": self.name,
            "description": (
                "Vote for the bot to ignore a normal IRC user. Use this when the "
                "requesting user clearly asks to vote to ignore, mute, or block a "
                "specific user from using the bot. Three distinct users must vote "
                "within a rolling 24-hour window. Admins and owners cannot be targeted."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "target_user": {
                        "type": "string",
                        "description": "The IRC nickname the requesting user wants the bot to ignore.",
                    }
                },
                "required": ["target_user"],
                "additionalProperties": False,
            },
        }

    @staticmethod
    def _validate_nick(nick: str) -> str:
        cleaned = (nick or "").strip()
        if (
            not cleaned
            or len(cleaned) > 64
            or cleaned[0] in "#&"
            or re.search(r"[\s\x00-\x1f]", cleaned)
        ):
            raise ValueError("Please provide a valid IRC nickname to vote on.")
        return cleaned

    def execute(
        self,
        target_user: str,
        requesting_user: str = "",
        _voter_identity: str = "",
        _current_network: str = "",
        _current_channel: str = "",
        **kwargs: Any,
    ) -> str:
        """Record one vote, applying the existing ignored level at the threshold."""
        try:
            target_display = self._validate_nick(target_user)
            voter_display = self._validate_nick(requesting_user)
        except ValueError as exc:
            return str(exc)

        target = target_display.casefold()
        voter = voter_display.casefold()
        if target == voter:
            return "You cannot vote to ignore yourself."
        if target in RESERVED_OWNER_NICKS:
            return (
                f"{target_display} is the bot owner and cannot be voted onto "
                "the ignore list."
            )

        # Prefer the server-supplied ident@host so changing nick does not create
        # extra votes. Only its digest is persisted; fall back to nick when the
        # IRC server did not provide a hostmask.
        voter_identity = (_voter_identity or f"nick:{voter}").strip().casefold()
        voter_key = hashlib.sha256(voter_identity.encode("utf-8")).hexdigest()

        now = int(self._now())
        cutoff = now - self.window_seconds
        connection = self._connect()
        try:
            connection.isolation_level = None
            connection.execute("BEGIN IMMEDIATE")
            connection.execute("DELETE FROM ignore_votes WHERE voted_at <= ?", (cutoff,))

            nick_row = connection.execute(
                "SELECT id, level FROM users WHERE nick = ? COLLATE NOCASE LIMIT 1",
                (target_display,),
            ).fetchone()

            # A registered admin/owner may currently be using another nick. Match
            # the channel tracker's server-supplied hostmask just as the Go
            # permission resolver does, so an alias cannot bypass immunity.
            identity_row = None
            if _current_network and _current_channel:
                tracked = connection.execute(
                    """
                    SELECT hostmask FROM channel_users
                    WHERE network = ? COLLATE NOCASE
                      AND channel = ? COLLATE NOCASE
                      AND nick = ? COLLATE NOCASE
                    LIMIT 1
                    """,
                    (_current_network, _current_channel, target_display),
                ).fetchone()
                if tracked is not None and tracked[0]:
                    identity_row = connection.execute(
                        "SELECT id, level FROM users WHERE hostmask = ? LIMIT 1",
                        (tracked[0],),
                    ).fetchone()

            matched_rows = [row for row in (nick_row, identity_row) if row is not None]
            if any(row[1] >= ADMIN_LEVEL for row in matched_rows):
                connection.execute("ROLLBACK")
                return (
                    f"{target_display} is an admin or owner and cannot be voted "
                    "onto the ignore list."
                )
            if any(row[1] == IGNORED_LEVEL for row in matched_rows):
                connection.execute("ROLLBACK")
                return f"{target_display} is already on the bot's ignore list."

            user_row = nick_row or identity_row

            duplicate = connection.execute(
                "SELECT 1 FROM ignore_votes WHERE target_nick = ? AND voter_key = ?",
                (target, voter_key),
            ).fetchone()
            if duplicate is not None:
                count = connection.execute(
                    "SELECT COUNT(*) FROM ignore_votes WHERE target_nick = ?",
                    (target,),
                ).fetchone()[0]
                connection.execute("COMMIT")
                return (
                    f"You already voted to ignore {target_display} during the current "
                    f"24-hour window ({count}/{self.threshold} votes)."
                )

            connection.execute(
                "INSERT INTO ignore_votes(target_nick, voter_key, voted_at) VALUES (?, ?, ?)",
                (target, voter_key, now),
            )
            count = connection.execute(
                "SELECT COUNT(*) FROM ignore_votes WHERE target_nick = ?",
                (target,),
            ).fetchone()[0]

            if count >= self.threshold:
                if user_row is None:
                    connection.execute(
                        """
                        INSERT INTO users(nick, hostmask, level, created_at, updated_at)
                        VALUES (?, '', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
                        """,
                        (target_display, IGNORED_LEVEL),
                    )
                else:
                    connection.execute(
                        "UPDATE users SET level = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
                        (IGNORED_LEVEL, user_row[0]),
                    )
                # The ballot has been acted on. Removing it prevents old votes from
                # immediately overriding a later admin decision to unignore the user.
                connection.execute("DELETE FROM ignore_votes WHERE target_nick = ?", (target,))
                connection.execute("COMMIT")
                return (
                    f"{target_display} reached {count} votes within 24 hours and has "
                    "been added to the bot's ignore list."
                )

            connection.execute("COMMIT")
            remaining = self.threshold - count
            vote_word = "vote" if remaining == 1 else "votes"
            return (
                f"Vote recorded for {target_display} ({count}/{self.threshold} in the "
                f"current 24-hour window; {remaining} more {vote_word} needed)."
            )
        except sqlite3.Error as exc:
            try:
                connection.execute("ROLLBACK")
            except sqlite3.Error:
                pass
            return f"Could not record the ignore vote: {exc}"
        finally:
            connection.close()
