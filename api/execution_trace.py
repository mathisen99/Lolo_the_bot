"""Private, sanitized execution traces for user-requested step summaries."""

from __future__ import annotations

import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


DEFAULT_TRACE_DB = Path(__file__).parent.parent / "data" / "execution_traces.db"
MAX_OBJECTIVE_LENGTH = 4000
MAX_FINAL_ANSWER_LENGTH = 12000
MAX_STEP_DETAILS_LENGTH = 8000
MAX_STEP_RESULT_LENGTH = 8000


class ExecutionTraceStore:
    """Persist a bounded audit of user requests and sanitized observable tool activity."""

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
                    status TEXT NOT NULL DEFAULT 'running',
                    objective TEXT NOT NULL DEFAULT '',
                    final_answer TEXT NOT NULL DEFAULT ''
                );
                CREATE TABLE IF NOT EXISTS execution_steps (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    request_id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    summary TEXT NOT NULL,
                    outcome TEXT NOT NULL,
                    created_at TEXT NOT NULL,
                    tool_name TEXT NOT NULL DEFAULT '',
                    details TEXT NOT NULL DEFAULT '',
                    result_summary TEXT NOT NULL DEFAULT '',
                    FOREIGN KEY (request_id) REFERENCES execution_runs(request_id)
                        ON DELETE CASCADE,
                    UNIQUE(request_id, position)
                );
                CREATE INDEX IF NOT EXISTS idx_execution_runs_scope
                    ON execution_runs(network, channel, nick, started_at DESC);
                """
            )

            # Existing installations have the original small schema. SQLite has
            # no ADD COLUMN IF NOT EXISTS, so migrate it by inspecting columns.
            self._ensure_column(connection, "execution_runs", "objective", "TEXT NOT NULL DEFAULT ''")
            self._ensure_column(connection, "execution_runs", "final_answer", "TEXT NOT NULL DEFAULT ''")
            self._ensure_column(connection, "execution_steps", "tool_name", "TEXT NOT NULL DEFAULT ''")
            self._ensure_column(connection, "execution_steps", "details", "TEXT NOT NULL DEFAULT ''")
            self._ensure_column(connection, "execution_steps", "result_summary", "TEXT NOT NULL DEFAULT ''")

    @staticmethod
    def _ensure_column(
        connection: sqlite3.Connection,
        table: str,
        column: str,
        definition: str,
    ) -> None:
        columns = {
            row["name"]
            for row in connection.execute(f"PRAGMA table_info({table})").fetchall()
        }
        if column not in columns:
            connection.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")

    @staticmethod
    def _now() -> str:
        return datetime.now(timezone.utc).isoformat(timespec="microseconds")

    @staticmethod
    def _clean(value: object, limit: int) -> str:
        text = str(value or "").replace("\x00", "")
        if len(text) <= limit:
            return text
        return text[:limit] + f"\n[truncated after {limit} characters]"

    def start_run(
        self,
        request_id: str,
        nick: str,
        network: str,
        channel: str,
        objective: str = "",
    ) -> None:
        with self._connect() as connection:
            connection.execute(
                """
                INSERT OR REPLACE INTO execution_runs
                    (request_id, nick, network, channel, started_at, finished_at,
                     status, objective, final_answer)
                VALUES (?, ?, ?, ?, ?, NULL, 'running', ?, '')
                """,
                (
                    request_id,
                    nick,
                    network or "libera",
                    channel or nick,
                    self._now(),
                    self._clean(objective, MAX_OBJECTIVE_LENGTH),
                ),
            )

    def append_step(
        self,
        request_id: str,
        summary: str,
        outcome: str = "completed",
        tool_name: str = "",
        details: str = "",
        result_summary: str = "",
    ) -> None:
        """Append an observable action with bounded, pre-sanitized evidence."""
        clean_summary = " ".join(str(summary).split())[:180]
        clean_outcome = " ".join(str(outcome).split())[:40]
        if not clean_summary:
            return
        with self._connect() as connection:
            connection.execute(
                """
                INSERT INTO execution_steps
                    (request_id, position, summary, outcome, created_at,
                     tool_name, details, result_summary)
                SELECT ?, COALESCE(MAX(position), 0) + 1, ?, ?, ?, ?, ?, ?
                FROM execution_steps WHERE request_id = ?
                """,
                (
                    request_id,
                    clean_summary,
                    clean_outcome,
                    self._now(),
                    self._clean(tool_name, 80),
                    self._clean(details, MAX_STEP_DETAILS_LENGTH),
                    self._clean(result_summary, MAX_STEP_RESULT_LENGTH),
                    request_id,
                ),
            )

    def finish_run(self, request_id: str, status: str, final_answer: str = "") -> None:
        with self._connect() as connection:
            connection.execute(
                """
                UPDATE execution_runs
                SET finished_at = ?,
                    status = CASE WHEN status = 'trace_lookup' THEN status ELSE ? END,
                    final_answer = ?
                WHERE request_id = ?
                """,
                (
                    self._now(),
                    status,
                    self._clean(final_answer, MAX_FINAL_ANSWER_LENGTH),
                    request_id,
                ),
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
        limit: int = 100,
    ) -> Optional[dict]:
        """Return the latest completed trace in the same user/channel/network scope."""
        with self._connect() as connection:
            run = connection.execute(
                """
                SELECT request_id, started_at, finished_at, status, objective, final_answer
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
                SELECT summary, outcome, tool_name, details, result_summary
                FROM execution_steps
                WHERE request_id = ?
                ORDER BY position ASC
                LIMIT ?
                """,
                (run["request_id"], max(1, min(limit, 100))),
            ).fetchall()

        return {
            "request_id": run["request_id"],
            "started_at": run["started_at"],
            "finished_at": run["finished_at"],
            "status": run["status"],
            "objective": run["objective"],
            "final_answer": run["final_answer"],
            "steps": [dict(step) for step in steps],
        }
