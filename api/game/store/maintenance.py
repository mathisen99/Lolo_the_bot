"""Bounded, transactional retention and authenticated deletion maintenance."""
from __future__ import annotations

import hashlib
import json
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from uuid import uuid4

from ..config import GameConfigSnapshot
from .sqlite import SQLiteConnectionPool


@dataclass(frozen=True)
class MaintenanceResult:
    run_id: str
    status: str
    deleted_sessions: int = 0
    deleted_action_records: int = 0
    deleted_archives: int = 0
    deleted_recovery_records: int = 0
    deleted_audits: int = 0
    deleted_tokens: int = 0
    deleted_menu_contexts: int = 0
    error_category: str | None = None


def _session_ref_hash(network: str, kind: str, value: str) -> bytes:
    return hashlib.sha256(f"{network}\x1f{kind}\x1f{value}".encode("utf-8")).digest()


class GameMaintenance:
    """Run one bounded cleanup transaction; failure preserves retained rows."""

    def __init__(self, pool: SQLiteConnectionPool, config: GameConfigSnapshot) -> None:
        self._pool = pool
        self._config = config

    def run_once(self, now: datetime | None = None) -> MaintenanceResult:
        now = now or datetime.now(timezone.utc)
        run_id = str(uuid4())
        started = now.isoformat()
        with self._pool.connection() as connection:
            connection.execute(
                "INSERT INTO maintenance_runs(run_id, kind, status, started_at, details_json) "
                "VALUES (?, 'retention', 'running', ?, '{}')",
                (run_id, started),
            )
            connection.commit()
        try:
            with self._pool.connection() as connection:
                connection.execute("BEGIN IMMEDIATE")
                result = self._cleanup(connection, run_id, now)
                connection.execute(
                    "UPDATE maintenance_runs SET status = 'completed', finished_at = ?, details_json = ? "
                    "WHERE run_id = ?",
                    (now.isoformat(), json.dumps(result.__dict__, sort_keys=True), run_id),
                )
                connection.commit()
                return result
        except (sqlite3.DatabaseError, OSError):
            # The cleanup transaction has rolled back. Recording this failure is
            # best-effort and never changes retained player records.
            try:
                with self._pool.connection() as connection:
                    connection.execute(
                        "UPDATE maintenance_runs SET status = 'failed', finished_at = ?, details_json = ? "
                        "WHERE run_id = ?",
                        (now.isoformat(), '{"error_category":"maintenance_failed"}', run_id),
                    )
                    connection.commit()
            except sqlite3.DatabaseError:
                pass
            return MaintenanceResult(run_id=run_id, status="failed", error_category="maintenance_failed")

    def _cleanup(self, connection: sqlite3.Connection, run_id: str, now: datetime) -> MaintenanceResult:
        limit = self._config.maintenance_batch_size
        now_text = now.isoformat()
        expired = connection.execute(
            "SELECT session_id, network_id, identity_kind, identity_value FROM game_sessions "
            "WHERE expires_at <= ? ORDER BY expires_at LIMIT ?",
            (now_text, limit),
        ).fetchall()
        for row in expired:
            session_id = str(row["session_id"])
            session_hash = _session_ref_hash(row["network_id"], row["identity_kind"], row["identity_value"])
            connection.execute("DELETE FROM game_audits WHERE session_ref_hash = ?", (session_hash,))
            connection.execute("DELETE FROM session_archives WHERE session_id = ?", (session_id,))
            connection.execute("DELETE FROM recovery_metadata WHERE session_id = ?", (session_id,))
            connection.execute(
                "DELETE FROM identity_links WHERE network_id = ? AND "
                "((source_kind = ? AND source_value = ?) OR (target_kind = ? AND target_value = ?))",
                (row["network_id"], row["identity_kind"], row["identity_value"], row["identity_kind"], row["identity_value"]),
            )
            connection.execute("DELETE FROM game_sessions WHERE session_id = ?", (session_id,))
            connection.execute(
                "INSERT INTO aggregate_metrics(metric_date, network_id, metric_name, value) "
                "VALUES (?, ?, 'expired_sessions_deleted', 1) "
                "ON CONFLICT(metric_date, network_id, metric_name) DO UPDATE SET value = value + 1",
                (now.date().isoformat(), row["network_id"]),
            )

        action_cutoff = (now - timedelta(days=self._config.action_record_retention_days)).isoformat()
        audit_cutoff = (now - timedelta(days=self._config.audit_retention_days)).isoformat()
        recovery_cutoff = (now - timedelta(days=self._config.recovery_snapshot_retention_days)).isoformat()
        token_cutoff = (now - timedelta(days=1)).isoformat()
        deleted_actions = self._delete_limited(connection, "action_results", "created_at <= ?", action_cutoff, limit)
        deleted_archives = self._delete_limited(connection, "session_archives", "expires_at <= ?", now_text, limit)
        deleted_recovery = self._delete_limited(
            connection, "recovery_metadata", "resolved_at IS NOT NULL AND resolved_at <= ?", recovery_cutoff, limit,
        )
        deleted_audits = self._delete_limited(connection, "game_audits", "created_at <= ?", audit_cutoff, limit)
        deleted_tokens = self._delete_limited(
            connection, "reset_tokens", "expires_at <= ? OR (used_at IS NOT NULL AND used_at <= ?)",
            (now_text, token_cutoff), limit,
        )
        deleted_contexts = self._delete_limited(
            connection, "menu_contexts", "expires_at <= ? OR (superseded_at IS NOT NULL AND superseded_at <= ?)",
            (now_text, token_cutoff), limit,
        )
        return MaintenanceResult(
            run_id=run_id,
            status="completed",
            deleted_sessions=len(expired),
            deleted_action_records=deleted_actions,
            deleted_archives=deleted_archives,
            deleted_recovery_records=deleted_recovery,
            deleted_audits=deleted_audits,
            deleted_tokens=deleted_tokens,
            deleted_menu_contexts=deleted_contexts,
        )

    @staticmethod
    def _delete_limited(
        connection: sqlite3.Connection,
        table: str,
        predicate: str,
        parameters: str | tuple[str, ...],
        limit: int,
    ) -> int:
        params = (parameters,) if isinstance(parameters, str) else parameters
        cursor = connection.execute(
            f"DELETE FROM {table} WHERE rowid IN (SELECT rowid FROM {table} WHERE {predicate} LIMIT ?)",
            (*params, limit),
        )
        return max(0, cursor.rowcount)

    def delete_authenticated_session(self, network: str, kind: str, value: str) -> bool:
        """Delete one identity after authentication by the trusted caller."""
        with self._pool.connection() as connection:
            connection.execute("BEGIN IMMEDIATE")
            try:
                row = connection.execute(
                    "SELECT session_id FROM game_sessions WHERE network_id = ? AND identity_kind = ? AND identity_value = ?",
                    (network, kind, value),
                ).fetchone()
                if row is None:
                    connection.commit()
                    return False
                session_id = str(row["session_id"])
                connection.execute("DELETE FROM game_audits WHERE session_ref_hash = ?", (_session_ref_hash(network, kind, value),))
                connection.execute("DELETE FROM session_archives WHERE session_id = ?", (session_id,))
                connection.execute("DELETE FROM recovery_metadata WHERE session_id = ?", (session_id,))
                connection.execute(
                    "DELETE FROM identity_links WHERE network_id = ? AND "
                    "((source_kind = ? AND source_value = ?) OR (target_kind = ? AND target_value = ?))",
                    (network, kind, value, kind, value),
                )
                connection.execute("DELETE FROM game_sessions WHERE session_id = ?", (session_id,))
                connection.commit()
                return True
            except Exception:
                connection.rollback()
                raise


__all__ = ["GameMaintenance", "MaintenanceResult"]
