"""Private, sanitized execution traces for user-requested step summaries."""

from __future__ import annotations

import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


DEFAULT_TRACE_DB = Path(__file__).parent.parent / "data" / "execution_traces.db"


class ExecutionTraceStore:
    """Persist a small audit trail without storing prompts, tool arguments, or results."""

    def __init__(self, db_path: Optional[Path] = None) -> None:
        self.db_path = Path(db_path) if db_path else DEFAULT_TRACE_DB
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self._initialize()

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(str(self.db_path), timeout=10)
        connection.row_factory = sqlite3.Row
        return connection

    def _initialize(self) -> None:
        with self._connect() as connection:
            connection.executescript(
                """
                PRAGMA journal_mode=WAL;
                CREATE TABLE IF NOT EXISTS execution_runs (
                    request_id TEXT PRIMARY KEY,
                    nick TEXT NOT NULL COLLATE NOCASE,
                    network TEXT NOT NULL COLLATE NOCASE,
                    channel TEXT NOT NULL COLLATE NOCASE,
                    started_at TEXT NOT NULL,
                    finished_at TEXT,
                    status TEXT NOT NULL DEFAULT 'running'
                );
                CREATE TABLE IF NOT EXISTS execution_steps (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    request_id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    summary TEXT NOT NULL,
                    outcome TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    FOREIGN KEY (request_id) REFERENCES execution_runs(request_id)
                        ON DELETE CASCADE,
                    UNIQUE(request_id, position)
                );
                CREATE INDEX IF NOT EXISTS idx_execution_runs_scope
                    ON execution_runs(network, channel, nick, started_at DESC);
                """
            )

    @staticmethod
    def _now() -> str:
        return datetime.now(timezone.utc).isoformat(timespec="microseconds")

    def start_run(self, request_id: str, nick: str, network: str, channel: str) -> None:
        with self._connect() as connection:
            connection.execute(
                """
                INSERT OR REPLACE INTO execution_runs
                    (request_id, nick, network, channel, started_at, finished_at, status)
                VALUES (?, ?, ?, ?, ?, NULL, 'running')
                """,
                (request_id, nick, network or "libera", channel or nick, self._now()),
            )

    def append_step(
        self,
        request_id: str,
        summary: str,
        outcome: str = "completed",
    ) -> None:
        """Append only a concise description and outcome; never raw tool data."""
        clean_summary = " ".join(str(summary).split())[:180]
        clean_outcome = " ".join(str(outcome).split())[:40]
        if not clean_summary:
            return
        with self._connect() as connection:
            connection.execute(
                """
                INSERT INTO execution_steps
                    (request_id, position, summary, outcome, created_at)
                SELECT ?, COALESCE(MAX(position), 0) + 1, ?, ?, ?
                FROM execution_steps WHERE request_id = ?
                """,
                (request_id, clean_summary, clean_outcome, self._now(), request_id),
            )

    def finish_run(self, request_id: str, status: str) -> None:
        with self._connect() as connection:
            connection.execute(
                """
                UPDATE execution_runs
                SET finished_at = ?,
                    status = CASE WHEN status = 'trace_lookup' THEN status ELSE ? END
                WHERE request_id = ?
                """,
                (self._now(), status, request_id),
            )

    def mark_trace_lookup(self, request_id: str) -> None:
        """Keep trace-display requests from hiding the answer they inspected."""
        if not request_id:
            return
        with self._connect() as connection:
            connection.execute(
                "UPDATE execution_runs SET status = 'trace_lookup' WHERE request_id = ?",
                (request_id,),
            )

    def latest_trace(
        self,
        nick: str,
        network: str,
        channel: str,
        exclude_request_id: str = "",
        limit: int = 12,
    ) -> Optional[dict]:
        """Return the latest completed trace in the same user/channel/network scope."""
        with self._connect() as connection:
            run = connection.execute(
                """
                SELECT request_id, started_at, status
                FROM execution_runs
                WHERE nick = ? COLLATE NOCASE
                  AND network = ? COLLATE NOCASE
                  AND channel = ? COLLATE NOCASE
                  AND request_id != ?
                  AND status NOT IN ('running', 'trace_lookup')
                ORDER BY started_at DESC
                LIMIT 1
                """,
                (nick, network or "libera", channel or nick, exclude_request_id),
            ).fetchone()
            if run is None:
                return None

            steps = connection.execute(
                """
                SELECT summary, outcome
                FROM execution_steps
                WHERE request_id = ?
                ORDER BY position ASC
                LIMIT ?
                """,
                (run["request_id"], max(1, min(limit, 20))),
            ).fetchall()

        return {
            "request_id": run["request_id"],
            "started_at": run["started_at"],
            "status": run["status"],
            "steps": [dict(step) for step in steps],
        }
