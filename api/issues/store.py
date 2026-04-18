"""SQLite storage for bug/feature reports and automation state."""

from __future__ import annotations

import sqlite3
from contextlib import contextmanager
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Iterator, List, Optional

from .models import (
    ALL_STATUSES,
    json_dumps,
    json_loads,
    normalize_issue_type,
    title_from_description,
    utc_now,
)


class IssueStore:
    """Compatibility-aware store backed by data/bugs.db."""

    DEFAULT_DB_PATH = Path(__file__).parent.parent.parent / "data" / "bugs.db"

    def __init__(self, db_path: Optional[Path] = None):
        self.db_path = Path(db_path) if db_path else self.DEFAULT_DB_PATH
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self.init_database()

    @contextmanager
    def _connect(self) -> Iterator[sqlite3.Connection]:
        conn = sqlite3.connect(str(self.db_path))
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA foreign_keys = ON")
        conn.execute("PRAGMA busy_timeout = 5000")
        try:
            yield conn
            conn.commit()
        except Exception:
            conn.rollback()
            raise
        finally:
            conn.close()

    def init_database(self) -> None:
        with self._connect() as conn:
            self._ensure_legacy_bugs_table(conn)
            self._ensure_workflow_tables(conn)
            self._sync_issue_sequence_with_legacy(conn)

    def _ensure_legacy_bugs_table(self, conn: sqlite3.Connection) -> None:
        conn.execute(
            """
            CREATE TABLE IF NOT EXISTS bugs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                reporter TEXT NOT NULL,
                channel TEXT,
                description TEXT NOT NULL,
                status TEXT DEFAULT 'open',
                priority TEXT DEFAULT 'normal',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                resolved_by TEXT,
                resolution_note TEXT
            )
            """
        )

    def _ensure_workflow_tables(self, conn: sqlite3.Connection) -> None:
        conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS issues (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                source_bug_id INTEGER UNIQUE,
                type TEXT NOT NULL DEFAULT 'bug',
                title TEXT NOT NULL,
                description TEXT NOT NULL,
                reporter TEXT NOT NULL,
                channel TEXT,
                status TEXT NOT NULL DEFAULT 'open',
                priority TEXT NOT NULL DEFAULT 'normal',
                labels_json TEXT NOT NULL DEFAULT '[]',
                acceptance_criteria_json TEXT NOT NULL DEFAULT '[]',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                closed_at TEXT,
                closed_by TEXT
            );

            CREATE TABLE IF NOT EXISTS issue_comments (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                author TEXT NOT NULL,
                kind TEXT NOT NULL DEFAULT 'comment',
                body TEXT NOT NULL,
                created_at TEXT NOT NULL,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS issue_plans (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                version INTEGER NOT NULL,
                summary TEXT NOT NULL,
                requested_change TEXT NOT NULL,
                affected_paths_json TEXT NOT NULL DEFAULT '[]',
                risk_level TEXT NOT NULL DEFAULT 'medium',
                test_plan_json TEXT NOT NULL DEFAULT '[]',
                rollback_plan TEXT NOT NULL DEFAULT '',
                automation_safe INTEGER NOT NULL DEFAULT 0,
                planner_model TEXT NOT NULL DEFAULT '',
                plan_text TEXT NOT NULL,
                plan_json TEXT NOT NULL,
                plan_hash TEXT NOT NULL,
                botbin_url TEXT,
                created_at TEXT NOT NULL,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
                UNIQUE(issue_id, plan_hash)
            );

            CREATE TABLE IF NOT EXISTS issue_approvals (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                plan_id INTEGER NOT NULL,
                plan_hash TEXT NOT NULL,
                decision TEXT NOT NULL,
                decided_by TEXT NOT NULL,
                note TEXT,
                created_at TEXT NOT NULL,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
                FOREIGN KEY(plan_id) REFERENCES issue_plans(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS issue_runs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                plan_id INTEGER NOT NULL,
                plan_hash TEXT NOT NULL,
                status TEXT NOT NULL,
                worker_id TEXT,
                lease_expires_at TEXT,
                started_at TEXT,
                finished_at TEXT,
                base_remote TEXT,
                base_branch TEXT,
                base_commit_sha TEXT,
                branch_name TEXT,
                worktree_path TEXT,
                commit_hash TEXT,
                pr_url TEXT,
                exit_code INTEGER,
                exit_summary TEXT,
                cancel_requested INTEGER NOT NULL DEFAULT 0,
                cancelled_at TEXT,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
                FOREIGN KEY(plan_id) REFERENCES issue_plans(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS issue_artifacts (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                run_id INTEGER,
                kind TEXT NOT NULL,
                path TEXT NOT NULL,
                botbin_url TEXT,
                summary TEXT,
                sha256 TEXT NOT NULL,
                created_at TEXT NOT NULL,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
                FOREIGN KEY(run_id) REFERENCES issue_runs(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS issue_locks (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                scope TEXT NOT NULL UNIQUE,
                owner TEXT NOT NULL,
                heartbeat_at TEXT NOT NULL,
                expires_at TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS issue_verifications (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                issue_id INTEGER NOT NULL,
                run_id INTEGER NOT NULL,
                check_name TEXT NOT NULL,
                command TEXT NOT NULL,
                status TEXT NOT NULL,
                output_artifact_id INTEGER,
                started_at TEXT NOT NULL,
                finished_at TEXT,
                FOREIGN KEY(issue_id) REFERENCES issues(id) ON DELETE CASCADE,
                FOREIGN KEY(run_id) REFERENCES issue_runs(id) ON DELETE CASCADE,
                FOREIGN KEY(output_artifact_id) REFERENCES issue_artifacts(id)
            );

            CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status);
            CREATE INDEX IF NOT EXISTS idx_issues_reporter ON issues(reporter);
            CREATE INDEX IF NOT EXISTS idx_issue_comments_issue ON issue_comments(issue_id);
            CREATE INDEX IF NOT EXISTS idx_issue_plans_issue ON issue_plans(issue_id);
            CREATE INDEX IF NOT EXISTS idx_issue_approvals_issue ON issue_approvals(issue_id);
            CREATE INDEX IF NOT EXISTS idx_issue_runs_issue ON issue_runs(issue_id);
            CREATE INDEX IF NOT EXISTS idx_issue_artifacts_issue ON issue_artifacts(issue_id);
            """
        )

    def _sync_issue_sequence_with_legacy(self, conn: sqlite3.Connection) -> None:
        legacy_max = conn.execute("SELECT COALESCE(MAX(id), 0) FROM bugs").fetchone()[0]
        issue_max = conn.execute("SELECT COALESCE(MAX(id), 0) FROM issues").fetchone()[0]
        if legacy_max <= issue_max:
            return
        conn.execute("DELETE FROM sqlite_sequence WHERE name = 'issues'")
        conn.execute("INSERT INTO sqlite_sequence(name, seq) VALUES('issues', ?)", (legacy_max,))

    def create_issue(
        self,
        issue_type: str,
        description: str,
        reporter: str,
        channel: str = "",
        priority: str = "normal",
        title: Optional[str] = None,
    ) -> int:
        issue_type = normalize_issue_type(issue_type, description)
        title = title or title_from_description(description)
        now = utc_now()
        with self._connect() as conn:
            cursor = conn.execute(
                """
                INSERT INTO issues (
                    type, title, description, reporter, channel, status, priority,
                    labels_json, acceptance_criteria_json, created_at, updated_at
                )
                VALUES (?, ?, ?, ?, ?, 'open', ?, '[]', '[]', ?, ?)
                """,
                (issue_type, title, description.strip(), reporter, channel, priority, now, now),
            )
            return int(cursor.lastrowid)

    def _row_to_dict(self, row: sqlite3.Row, source: str = "issue") -> Dict[str, Any]:
        data = dict(row)
        data["source"] = source
        if "labels_json" in data:
            data["labels"] = json_loads(data.get("labels_json"), [])
        if "acceptance_criteria_json" in data:
            data["acceptance_criteria"] = json_loads(data.get("acceptance_criteria_json"), [])
        return data

    def _legacy_row_to_issue_dict(self, row: sqlite3.Row) -> Dict[str, Any]:
        data = dict(row)
        description = data["description"]
        return {
            "id": data["id"],
            "source_bug_id": data["id"],
            "type": normalize_issue_type(None, description),
            "title": title_from_description(description),
            "description": description,
            "reporter": data["reporter"],
            "channel": data["channel"] or "",
            "status": data["status"],
            "priority": data["priority"],
            "labels": [],
            "acceptance_criteria": [],
            "created_at": data["created_at"],
            "updated_at": data["updated_at"],
            "closed_at": None,
            "closed_by": data.get("resolved_by") if hasattr(data, "get") else None,
            "resolved_by": data["resolved_by"],
            "resolution_note": data["resolution_note"],
            "source": "legacy_bug",
        }

    def get_issue(self, issue_id: int) -> Optional[Dict[str, Any]]:
        with self._connect() as conn:
            row = conn.execute("SELECT * FROM issues WHERE id = ?", (issue_id,)).fetchone()
            if row:
                return self._row_to_dict(row)
            legacy = conn.execute("SELECT * FROM bugs WHERE id = ?", (issue_id,)).fetchone()
            if legacy:
                return self._legacy_row_to_issue_dict(legacy)
        return None

    def ensure_issue(self, issue_id: int) -> Optional[Dict[str, Any]]:
        with self._connect() as conn:
            row = conn.execute("SELECT * FROM issues WHERE id = ?", (issue_id,)).fetchone()
            if row:
                return self._row_to_dict(row)

            legacy = conn.execute("SELECT * FROM bugs WHERE id = ?", (issue_id,)).fetchone()
            if not legacy:
                return None

            now = utc_now()
            description = legacy["description"]
            issue_type = normalize_issue_type(None, description)
            try:
                conn.execute(
                    """
                    INSERT INTO issues (
                        id, source_bug_id, type, title, description, reporter, channel,
                        status, priority, labels_json, acceptance_criteria_json,
                        created_at, updated_at
                    )
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', '[]', ?, ?)
                    """,
                    (
                        legacy["id"],
                        legacy["id"],
                        issue_type,
                        title_from_description(description),
                        description,
                        legacy["reporter"],
                        legacy["channel"] or "",
                        legacy["status"],
                        legacy["priority"],
                        legacy["created_at"] or now,
                        legacy["updated_at"] or now,
                    ),
                )
            except sqlite3.IntegrityError:
                conn.execute(
                    """
                    INSERT INTO issues (
                        source_bug_id, type, title, description, reporter, channel,
                        status, priority, labels_json, acceptance_criteria_json,
                        created_at, updated_at
                    )
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, '[]', '[]', ?, ?)
                    """,
                    (
                        legacy["id"],
                        issue_type,
                        title_from_description(description),
                        description,
                        legacy["reporter"],
                        legacy["channel"] or "",
                        legacy["status"],
                        legacy["priority"],
                        legacy["created_at"] or now,
                        legacy["updated_at"] or now,
                    ),
                )
            imported = conn.execute("SELECT * FROM issues WHERE source_bug_id = ?", (legacy["id"],)).fetchone()
            return self._row_to_dict(imported) if imported else None

    def list_reports(self, requester: str, permission_level: str, filter_status: str = "open") -> List[Dict[str, Any]]:
        params: List[Any] = []
        issue_where = []
        legacy_where = [
            "NOT EXISTS (SELECT 1 FROM issues WHERE issues.source_bug_id = bugs.id)"
        ]

        if permission_level not in ("owner", "admin"):
            issue_where.append("reporter = ?")
            legacy_where.append("reporter = ?")
            params.append(requester)

        issue_params = list(params)
        legacy_params = list(params)

        if filter_status != "all":
            issue_where.append("status = ?")
            legacy_where.append("status = ?")
            issue_params.append(filter_status)
            legacy_params.append(filter_status)

        issue_sql = "SELECT * FROM issues"
        if issue_where:
            issue_sql += " WHERE " + " AND ".join(issue_where)
        issue_sql += " ORDER BY created_at DESC LIMIT 20"

        legacy_sql = "SELECT * FROM bugs"
        if legacy_where:
            legacy_sql += " WHERE " + " AND ".join(legacy_where)
        legacy_sql += " ORDER BY created_at DESC LIMIT 20"

        with self._connect() as conn:
            reports = [self._row_to_dict(row) for row in conn.execute(issue_sql, issue_params).fetchall()]
            reports.extend(
                self._legacy_row_to_issue_dict(row)
                for row in conn.execute(legacy_sql, legacy_params).fetchall()
            )

        reports.sort(key=lambda item: item.get("created_at") or "", reverse=True)
        return reports[:20 if permission_level in ("owner", "admin") else 10]

    def can_view(self, issue: Dict[str, Any], requester: str, permission_level: str) -> bool:
        return permission_level in ("owner", "admin") or issue.get("reporter") == requester

    def update_report(self, issue_id: int, status: Optional[str], priority: Optional[str]) -> bool:
        if status and status not in ALL_STATUSES:
            raise ValueError(f"invalid status: {status}")
        updates = []
        params: List[Any] = []
        if status:
            updates.append("status = ?")
            params.append(status)
        if priority:
            updates.append("priority = ?")
            params.append(priority)
        if not updates:
            raise ValueError("no updates specified")
        updates.append("updated_at = ?")
        params.append(utc_now())
        params.append(issue_id)

        with self._connect() as conn:
            cur = conn.execute(f"UPDATE issues SET {', '.join(updates)} WHERE id = ?", params)
            if cur.rowcount:
                return True
            cur = conn.execute(f"UPDATE bugs SET {', '.join(updates)} WHERE id = ?", params)
            return bool(cur.rowcount)

    def resolve_report(self, issue_id: int, resolver: str, note: str = "") -> bool:
        now = utc_now()
        with self._connect() as conn:
            cur = conn.execute(
                """
                UPDATE issues
                SET status = 'resolved', closed_by = ?, closed_at = ?, updated_at = ?
                WHERE id = ?
                """,
                (resolver, now, now, issue_id),
            )
            if cur.rowcount:
                if note:
                    conn.execute(
                        "INSERT INTO issue_comments(issue_id, author, kind, body, created_at) VALUES (?, ?, 'resolution', ?, ?)",
                        (issue_id, resolver, note, now),
                    )
                return True
            cur = conn.execute(
                """
                UPDATE bugs
                SET status = 'resolved', resolved_by = ?, resolution_note = ?, updated_at = ?
                WHERE id = ?
                """,
                (resolver, note or "Resolved", now, issue_id),
            )
            return bool(cur.rowcount)

    def delete_report(self, issue_id: int) -> bool:
        with self._connect() as conn:
            cur = conn.execute("DELETE FROM issues WHERE id = ?", (issue_id,))
            if cur.rowcount:
                return True
            cur = conn.execute("DELETE FROM bugs WHERE id = ?", (issue_id,))
            return bool(cur.rowcount)

    def add_comment(self, issue_id: int, author: str, body: str, kind: str = "comment") -> int:
        issue = self.ensure_issue(issue_id)
        if not issue:
            raise ValueError(f"report #{issue_id} not found")
        now = utc_now()
        with self._connect() as conn:
            cur = conn.execute(
                "INSERT INTO issue_comments(issue_id, author, kind, body, created_at) VALUES (?, ?, ?, ?, ?)",
                (int(issue["id"]), author, kind, body.strip(), now),
            )
            conn.execute("UPDATE issues SET updated_at = ? WHERE id = ?", (now, int(issue["id"])))
            return int(cur.lastrowid)

    def list_comments(self, issue_id: int, limit: int = 10) -> List[Dict[str, Any]]:
        with self._connect() as conn:
            rows = conn.execute(
                "SELECT * FROM issue_comments WHERE issue_id = ? ORDER BY created_at DESC LIMIT ?",
                (issue_id, limit),
            ).fetchall()
            return [dict(row) for row in rows]

    def create_plan(
        self,
        issue_id: int,
        plan: Dict[str, Any],
        plan_hash: str,
        plan_text: str,
        planner_model: str = "codex",
        botbin_url: Optional[str] = None,
    ) -> int:
        now = utc_now()
        with self._connect() as conn:
            version = conn.execute(
                "SELECT COALESCE(MAX(version), 0) + 1 FROM issue_plans WHERE issue_id = ?",
                (issue_id,),
            ).fetchone()[0]
            values = (
                issue_id,
                version,
                plan.get("summary", ""),
                plan.get("requested_change", ""),
                json_dumps(plan.get("affected_paths") or []),
                plan.get("risk_level", "medium"),
                json_dumps(plan.get("test_plan") or []),
                plan.get("rollback_plan", ""),
                1 if plan.get("automation_safe") else 0,
                planner_model,
                plan_text,
                json_dumps(plan),
                plan_hash,
                botbin_url,
                now,
            )
            try:
                cur = conn.execute(
                    """
                    INSERT INTO issue_plans (
                        issue_id, version, summary, requested_change, affected_paths_json,
                        risk_level, test_plan_json, rollback_plan, automation_safe,
                        planner_model, plan_text, plan_json, plan_hash, botbin_url, created_at
                    )
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    values,
                )
                plan_id = int(cur.lastrowid)
            except sqlite3.IntegrityError:
                row = conn.execute(
                    "SELECT id FROM issue_plans WHERE issue_id = ? AND plan_hash = ?",
                    (issue_id, plan_hash),
                ).fetchone()
                if not row:
                    raise
                plan_id = int(row["id"])
                conn.execute(
                    "UPDATE issue_plans SET plan_text = ?, plan_json = ?, botbin_url = ? WHERE id = ?",
                    (plan_text, json_dumps(plan), botbin_url, plan_id),
                )
            approved = conn.execute(
                "SELECT 1 FROM issue_approvals WHERE plan_id = ? AND decision = 'approved' LIMIT 1",
                (plan_id,),
            ).fetchone()
            issue_status = "approved" if approved else "approval_pending"
            conn.execute("UPDATE issues SET status = ?, updated_at = ? WHERE id = ?", (issue_status, now, issue_id))
            return plan_id

    def get_latest_plan(self, issue_id: int) -> Optional[Dict[str, Any]]:
        with self._connect() as conn:
            row = conn.execute(
                "SELECT * FROM issue_plans WHERE issue_id = ? ORDER BY version DESC, id DESC LIMIT 1",
                (issue_id,),
            ).fetchone()
            if not row:
                return None
            data = dict(row)
            data["affected_paths"] = json_loads(data.get("affected_paths_json"), [])
            data["test_plan"] = json_loads(data.get("test_plan_json"), [])
            data["plan"] = json_loads(data.get("plan_json"), {})
            data["automation_safe"] = bool(data.get("automation_safe"))
            return data

    def list_pending_plans(self) -> List[Dict[str, Any]]:
        with self._connect() as conn:
            rows = conn.execute(
                """
                SELECT p.*, i.title, i.reporter
                FROM issue_plans p
                JOIN issues i ON i.id = p.issue_id
                WHERE NOT EXISTS (
                    SELECT 1 FROM issue_approvals a WHERE a.plan_id = p.id
                )
                ORDER BY p.created_at DESC
                """
            ).fetchall()
            return [dict(row) for row in rows]

    def record_decision(
        self,
        issue_id: int,
        plan_id: int,
        plan_hash: str,
        decision: str,
        decided_by: str,
        note: str = "",
    ) -> int:
        now = utc_now()
        with self._connect() as conn:
            cur = conn.execute(
                """
                INSERT INTO issue_approvals(issue_id, plan_id, plan_hash, decision, decided_by, note, created_at)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """,
                (issue_id, plan_id, plan_hash, decision, decided_by, note, now),
            )
            new_status = "approved" if decision == "approved" else "rejected"
            conn.execute("UPDATE issues SET status = ?, updated_at = ? WHERE id = ?", (new_status, now, issue_id))
            return int(cur.lastrowid)

    def has_approval(self, issue_id: int, plan_id: int, plan_hash: str) -> bool:
        with self._connect() as conn:
            row = conn.execute(
                """
                SELECT 1 FROM issue_approvals
                WHERE issue_id = ? AND plan_id = ? AND plan_hash = ? AND decision = 'approved'
                ORDER BY created_at DESC LIMIT 1
                """,
                (issue_id, plan_id, plan_hash),
            ).fetchone()
            return bool(row)

    def create_run(self, issue_id: int, plan_id: int, plan_hash: str, status: str = "running") -> int:
        now = utc_now()
        with self._connect() as conn:
            cur = conn.execute(
                """
                INSERT INTO issue_runs(issue_id, plan_id, plan_hash, status, started_at)
                VALUES (?, ?, ?, ?, ?)
                """,
                (issue_id, plan_id, plan_hash, status, now),
            )
            conn.execute("UPDATE issues SET status = 'implementing', updated_at = ? WHERE id = ?", (now, issue_id))
            return int(cur.lastrowid)

    def update_run(self, run_id: int, **fields: Any) -> None:
        if not fields:
            return
        updates = []
        params: List[Any] = []
        for key, value in fields.items():
            updates.append(f"{key} = ?")
            params.append(value)
        params.append(run_id)
        with self._connect() as conn:
            conn.execute(f"UPDATE issue_runs SET {', '.join(updates)} WHERE id = ?", params)

    def add_artifact(
        self,
        issue_id: int,
        run_id: Optional[int],
        kind: str,
        path: str,
        sha256: str,
        summary: str = "",
        botbin_url: Optional[str] = None,
    ) -> int:
        with self._connect() as conn:
            cur = conn.execute(
                """
                INSERT INTO issue_artifacts(issue_id, run_id, kind, path, botbin_url, summary, sha256, created_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (issue_id, run_id, kind, path, botbin_url, summary, sha256, utc_now()),
            )
            return int(cur.lastrowid)

    def list_artifacts(self, issue_id: int, run_id: Optional[int] = None, limit: int = 10) -> List[Dict[str, Any]]:
        params: List[Any] = [issue_id]
        sql = "SELECT * FROM issue_artifacts WHERE issue_id = ?"
        if run_id is not None:
            sql += " AND run_id = ?"
            params.append(run_id)
        sql += " ORDER BY created_at DESC LIMIT ?"
        params.append(limit)
        with self._connect() as conn:
            return [dict(row) for row in conn.execute(sql, params).fetchall()]

    def get_latest_run(self, issue_id: int) -> Optional[Dict[str, Any]]:
        with self._connect() as conn:
            row = conn.execute(
                "SELECT * FROM issue_runs WHERE issue_id = ? ORDER BY id DESC LIMIT 1",
                (issue_id,),
            ).fetchone()
            return dict(row) if row else None

    def request_cancel(self, issue_id: int) -> bool:
        now = utc_now()
        with self._connect() as conn:
            row = conn.execute(
                "SELECT id FROM issue_runs WHERE issue_id = ? ORDER BY id DESC LIMIT 1",
                (issue_id,),
            ).fetchone()
            if not row:
                return False
            conn.execute(
                """
                UPDATE issue_runs
                SET cancel_requested = 1, cancelled_at = ?, status = 'cancel_requested'
                WHERE id = ?
                """,
                (now, row["id"]),
            )
            conn.execute("UPDATE issues SET status = 'cancel_requested', updated_at = ? WHERE id = ?", (now, issue_id))
            return True

    def is_cancel_requested(self, run_id: int) -> bool:
        with self._connect() as conn:
            row = conn.execute("SELECT cancel_requested FROM issue_runs WHERE id = ?", (run_id,)).fetchone()
            return bool(row and row["cancel_requested"])

    def add_verification(
        self,
        issue_id: int,
        run_id: int,
        check_name: str,
        command: str,
        status: str,
        artifact_id: Optional[int] = None,
    ) -> int:
        now = utc_now()
        with self._connect() as conn:
            cur = conn.execute(
                """
                INSERT INTO issue_verifications(
                    issue_id, run_id, check_name, command, status,
                    output_artifact_id, started_at, finished_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (issue_id, run_id, check_name, command, status, artifact_id, now, now),
            )
            return int(cur.lastrowid)

    def acquire_lock(self, scope: str, owner: str, ttl_seconds: int = 3600) -> bool:
        now = utc_now()
        expires = (datetime.now(UTC).replace(tzinfo=None) + timedelta(seconds=ttl_seconds)).isoformat()
        with self._connect() as conn:
            conn.execute("DELETE FROM issue_locks WHERE expires_at < ?", (now,))
            try:
                conn.execute(
                    """
                    INSERT INTO issue_locks(scope, owner, heartbeat_at, expires_at)
                    VALUES (?, ?, ?, ?)
                    """,
                    (scope, owner, now, expires),
                )
                return True
            except sqlite3.IntegrityError:
                return False

    def release_lock(self, scope: str, owner: Optional[str] = None) -> None:
        with self._connect() as conn:
            if owner:
                conn.execute("DELETE FROM issue_locks WHERE scope = ? AND owner = ?", (scope, owner))
            else:
                conn.execute("DELETE FROM issue_locks WHERE scope = ?", (scope,))
