"""
Bug and feature workflow tool.

This keeps the public tool name `bug_report` for compatibility, while expanding
the backend from a simple report list into an owner-approved Codex automation
workflow.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Dict, Optional

from .base import Tool
from api.issues.artifacts import ArtifactManager
from api.issues.models import ALL_STATUSES, normalize_issue_type
from api.issues.planner import CodexPlanner, PlannerError
from api.issues.store import IssueStore
from api.issues.worker import IssueWorker, WorkerError
from api.utils.output import log_info, log_success, log_error, log_warning


class BugReportTool(Tool):
    """Tool for managing bug reports, feature requests, plans, and owner-approved runs."""

    DB_PATH = Path(__file__).parent.parent.parent / "data" / "bugs.db"

    def __init__(
        self,
        store: Optional[IssueStore] = None,
        planner: Optional[CodexPlanner] = None,
        worker: Optional[IssueWorker] = None,
        artifacts: Optional[ArtifactManager] = None,
    ):
        self.store = store or IssueStore(self.DB_PATH)
        self.artifacts = artifacts or ArtifactManager(self.store)
        self.planner = planner or CodexPlanner()
        self.worker = worker or IssueWorker(self.store, artifacts=self.artifacts)

    @property
    def name(self) -> str:
        return "bug_report"

    def get_definition(self) -> Dict[str, Any]:
        return {
            "type": "function",
            "name": "bug_report",
            "description": """Manage Lolo bug reports and feature requests through natural language.

Use this for both normal user reports and owner automation:
- report: Submit a bug, feature request, or chore.
- list: List reports. Normal users see their own; owner/admin sees all.
- show: Show one report.
- comment: Add repro details, notes, or owner comments.
- update/delete: Current admin/owner management actions.
- resolve: Mark resolved. Reporter can resolve own report; owner/admin can resolve any.
- plan: OWNER ONLY. Ask local Codex for a read-only implementation plan and paste it to botbin.
- approve_plan/reject_plan: OWNER ONLY. Decide a pending plan by exact hash, or only if exactly one plan is pending.
- run: OWNER ONLY. Run approved Codex implementation in an isolated worktree.
- cancel/artifacts/status: OWNER ONLY for cancel, owner/admin for artifacts/status.

Never use plan/run for normal users. Never run code without owner approval.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": [
                            "report",
                            "list",
                            "show",
                            "comment",
                            "update",
                            "delete",
                            "resolve",
                            "plan",
                            "approve_plan",
                            "reject_plan",
                            "run",
                            "cancel",
                            "artifacts",
                            "status",
                        ],
                        "description": "Action to perform",
                    },
                    "issue_type": {
                        "type": ["string", "null"],
                        "enum": ["bug", "feature", "chore", None],
                        "description": "Report type for report action. Infer from description if omitted.",
                    },
                    "title": {
                        "type": ["string", "null"],
                        "description": "Optional short title for report action.",
                    },
                    "description": {
                        "type": ["string", "null"],
                        "description": "Bug/feature description for report action.",
                    },
                    "bug_id": {
                        "type": ["integer", "null"],
                        "description": "Report ID for show/comment/update/delete/resolve/plan/approve/reject/run/status/artifacts.",
                    },
                    "comment": {
                        "type": ["string", "null"],
                        "description": "Comment or note for comment/approve/reject actions.",
                    },
                    "status": {
                        "type": ["string", "null"],
                        "enum": sorted(ALL_STATUSES) + [None],
                        "description": "New status for update action.",
                    },
                    "priority": {
                        "type": ["string", "null"],
                        "enum": ["low", "normal", "high", "critical", None],
                        "description": "Priority for report/update actions.",
                    },
                    "resolution_note": {
                        "type": ["string", "null"],
                        "description": "Resolution note for resolve action.",
                    },
                    "filter_status": {
                        "type": ["string", "null"],
                        "enum": ["all"] + sorted(ALL_STATUSES) + [None],
                        "description": "Filter reports by status for list action. Default: open.",
                    },
                    "plan_hash": {
                        "type": ["string", "null"],
                        "description": "Plan hash for approve_plan/reject_plan. Required unless exactly one plan is pending.",
                    },
                    "run_id": {
                        "type": ["integer", "null"],
                        "description": "Optional run ID for artifacts action.",
                    },
                },
                "required": ["action"],
                "additionalProperties": False,
            },
        }

    def execute(
        self,
        action: str,
        description: Optional[str] = None,
        bug_id: Optional[int] = None,
        status: Optional[str] = None,
        priority: Optional[str] = None,
        resolution_note: Optional[str] = None,
        filter_status: str = "open",
        permission_level: str = "normal",
        requesting_user: str = "unknown",
        channel: str = "",
        issue_type: Optional[str] = None,
        title: Optional[str] = None,
        comment: Optional[str] = None,
        plan_hash: Optional[str] = None,
        run_id: Optional[int] = None,
        **kwargs: Any,
    ) -> str:
        action = (action or "").strip()
        filter_status = filter_status or "open"

        try:
            if action == "report":
                return self._report(requesting_user, channel, description or "", issue_type, title, priority or "normal")
            if action == "list":
                return self._list(requesting_user, permission_level, filter_status)
            if action == "show":
                return self._show_required(bug_id, requesting_user, permission_level)
            if action == "comment":
                return self._comment_required(bug_id, comment or description or "", requesting_user, permission_level)
            if action == "update":
                return self._update_required(bug_id, status, priority, requesting_user, permission_level)
            if action == "resolve":
                return self._resolve_required(bug_id, resolution_note or comment or "", requesting_user, permission_level)
            if action == "delete":
                return self._delete_required(bug_id, requesting_user, permission_level)
            if action == "plan":
                return self._plan_required(bug_id, requesting_user, permission_level)
            if action == "approve_plan":
                return self._decide_plan(bug_id, plan_hash, "approved", requesting_user, permission_level, comment or "")
            if action == "reject_plan":
                return self._decide_plan(bug_id, plan_hash, "rejected", requesting_user, permission_level, comment or "")
            if action == "run":
                return self._run_required(bug_id, requesting_user, permission_level)
            if action == "cancel":
                return self._cancel_required(bug_id, requesting_user, permission_level)
            if action == "artifacts":
                return self._artifacts_required(bug_id, run_id, requesting_user, permission_level)
            if action == "status":
                return self._status_required(bug_id, requesting_user, permission_level)
            return f"Error: Unknown action '{action}'."
        except Exception as exc:
            log_error(f"[BUG_REPORT] {action} failed: {exc}")
            return f"Error: {exc}"

    def _report(
        self,
        reporter: str,
        channel: str,
        description: str,
        issue_type: Optional[str],
        title: Optional[str],
        priority: str,
    ) -> str:
        if not description or len(description.strip()) < 10:
            return "Error: Bug/feature description must be at least 10 characters."
        normalized_type = normalize_issue_type(issue_type, description)
        issue_id = self.store.create_issue(normalized_type, description, reporter, channel, priority, title)
        log_success(f"[BUG_REPORT] New {normalized_type} #{issue_id} reported by {reporter}")
        label = "Feature request" if normalized_type == "feature" else "Bug report" if normalized_type == "bug" else "Chore report"
        return f"{label} #{issue_id} submitted successfully. Thank you for reporting!"

    def _list(self, requester: str, permission_level: str, filter_status: str) -> str:
        reports = self.store.list_reports(requester, permission_level, filter_status)
        if not reports:
            if permission_level in ("owner", "admin"):
                return f"No bug/feature reports found with status '{filter_status}'."
            return "You haven't submitted any bug/feature reports."
        header = f"Reports ({filter_status}):" if permission_level in ("owner", "admin") else "Your reports:"
        return header + " | " + " | ".join(self._format_summary(report) for report in reports)

    def _format_summary(self, report: Dict[str, Any]) -> str:
        created = str(report.get("created_at") or "")[:10]
        desc = " ".join(str(report.get("description") or "").split())
        if len(desc) > 60:
            desc = desc[:57].rstrip() + "..."
        return (
            f"#{report['id']} [{report.get('type', 'bug')}/{report.get('status', 'open')}/"
            f"{report.get('priority', 'normal')}] by {report.get('reporter', '?')} ({created}): {desc}"
        )

    def _require_bug_id(self, bug_id: Optional[int]) -> int:
        if not bug_id:
            raise ValueError("Please specify a report ID.")
        return int(bug_id)

    def _get_visible_issue(self, bug_id: int, requester: str, permission_level: str) -> Dict[str, Any]:
        issue = self.store.get_issue(bug_id)
        if not issue:
            raise ValueError(f"Report #{bug_id} not found.")
        if not self.store.can_view(issue, requester, permission_level):
            raise PermissionError("Permission denied: You can only view your own reports.")
        return issue

    def _show_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        issue = self._get_visible_issue(self._require_bug_id(bug_id), requester, permission_level)
        comments = self.store.list_comments(int(issue["id"]), limit=5) if issue.get("source") != "legacy_bug" else []
        latest_plan = self.store.get_latest_plan(int(issue["id"])) if issue.get("source") != "legacy_bug" else None
        parts = [
            self._format_summary(issue),
            f"Channel: {issue.get('channel') or '-'}",
        ]
        if comments:
            parts.append("Recent comments: " + " / ".join(f"{c['author']}: {c['body'][:80]}" for c in comments))
        if latest_plan:
            parts.append(f"Latest plan: hash {latest_plan['plan_hash']} automation_safe={bool(latest_plan['automation_safe'])}")
            if latest_plan.get("botbin_url"):
                parts.append(f"Plan: {latest_plan['botbin_url']}")
        return " | ".join(parts)

    def _comment_required(self, bug_id: Optional[int], body: str, requester: str, permission_level: str) -> str:
        issue_id = self._require_bug_id(bug_id)
        issue = self._get_visible_issue(issue_id, requester, permission_level)
        if not body.strip():
            return "Error: Please provide a comment."
        comment_id = self.store.add_comment(int(issue["id"]), requester, body)
        log_info(f"[BUG_REPORT] Comment #{comment_id} added to report #{issue['id']} by {requester}")
        return f"Comment added to report #{issue['id']}."

    def _update_required(
        self,
        bug_id: Optional[int],
        status: Optional[str],
        priority: Optional[str],
        requester: str,
        permission_level: str,
    ) -> str:
        if permission_level not in ("owner", "admin"):
            return "Permission denied: Only admins and owners can update reports."
        issue_id = self._require_bug_id(bug_id)
        if not self.store.update_report(issue_id, status, priority):
            return f"Error: Report #{issue_id} not found."
        log_info(f"[BUG_REPORT] Report #{issue_id} updated by {requester}")
        return f"Report #{issue_id} updated successfully."

    def _resolve_required(self, bug_id: Optional[int], note: str, requester: str, permission_level: str) -> str:
        issue_id = self._require_bug_id(bug_id)
        issue = self.store.get_issue(issue_id)
        if not issue:
            return f"Error: Report #{issue_id} not found."
        if permission_level not in ("owner", "admin") and requester != issue.get("reporter"):
            return f"Permission denied: Only the reporter ({issue.get('reporter')}) or an admin can resolve this report."
        if not self.store.resolve_report(issue_id, requester, note):
            return f"Error: Report #{issue_id} not found."
        log_success(f"[BUG_REPORT] Report #{issue_id} resolved by {requester}")
        return f"Report #{issue_id} marked as resolved."

    def _delete_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        if permission_level not in ("owner", "admin"):
            return "Permission denied: Only admins and owners can delete reports."
        issue_id = self._require_bug_id(bug_id)
        if not self.store.delete_report(issue_id):
            return f"Error: Report #{issue_id} not found."
        log_warning(f"[BUG_REPORT] Report #{issue_id} deleted by {requester}")
        return f"Report #{issue_id} deleted."

    def _owner_only(self, permission_level: str) -> Optional[str]:
        if permission_level != "owner":
            return "Permission denied: This workflow action is restricted to the bot owner only."
        return None

    def _plan_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        denied = self._owner_only(permission_level)
        if denied:
            return denied
        issue_id = self._require_bug_id(bug_id)
        issue = self.store.ensure_issue(issue_id)
        if not issue:
            return f"Error: Report #{issue_id} not found."
        comments = self.store.list_comments(int(issue["id"]), limit=20)
        try:
            plan = self.planner.plan(issue, comments)
        except PlannerError as exc:
            return f"Error: Codex planning failed: {exc}"

        plan_text = self._plan_text_with_hash(plan)
        _, path, botbin_url, upload_error = self.artifacts.write_text(
            int(issue["id"]),
            None,
            "codex-plan",
            plan_text,
            suffix=".md",
            upload=True,
            summary="Codex implementation plan",
        )
        plan_id = self.store.create_plan(
            int(issue["id"]),
            plan,
            plan["plan_hash"],
            plan_text,
            planner_model="codex",
            botbin_url=botbin_url,
        )
        log_success(f"[BUG_REPORT] Plan #{plan_id} created for report #{issue['id']} by {requester}")
        if botbin_url:
            return f"Plan ready for report #{issue['id']}: {botbin_url} hash {plan['plan_hash']}. Owner can approve or reject this plan."
        return (
            f"Plan ready for report #{issue['id']} with hash {plan['plan_hash']}, but botbin upload failed: "
            f"{upload_error}. Local artifact: {path}"
        )

    def _plan_text_with_hash(self, plan: Dict[str, Any]) -> str:
        return f"{plan.get('plan_text', '').strip()}\n\nPlan hash: {plan['plan_hash']}\n"

    def _resolve_pending_plan(self, bug_id: Optional[int], plan_hash: Optional[str]) -> Dict[str, Any]:
        if bug_id and plan_hash:
            plan = self.store.get_latest_plan(int(bug_id))
            if not plan:
                raise ValueError(f"Report #{bug_id} has no plan.")
            if str(plan.get("plan_hash")) != str(plan_hash):
                raise ValueError(f"Plan hash mismatch. Latest hash is {plan.get('plan_hash')}.")
            return plan

        pending = self.store.list_pending_plans()
        if bug_id:
            pending = [item for item in pending if int(item["issue_id"]) == int(bug_id)]
        if len(pending) != 1:
            raise ValueError("Approval/rejection is ambiguous. Specify the report ID and exact plan_hash.")
        plan = pending[0]
        if plan_hash and str(plan["plan_hash"]) != str(plan_hash):
            raise ValueError(f"Plan hash mismatch. Pending hash is {plan['plan_hash']}.")
        return plan

    def _decide_plan(
        self,
        bug_id: Optional[int],
        plan_hash: Optional[str],
        decision: str,
        requester: str,
        permission_level: str,
        note: str,
    ) -> str:
        denied = self._owner_only(permission_level)
        if denied:
            return denied
        plan = self._resolve_pending_plan(bug_id, plan_hash)
        approval_id = self.store.record_decision(
            int(plan["issue_id"]),
            int(plan["id"]),
            str(plan["plan_hash"]),
            decision,
            requester,
            note,
        )
        log_success(f"[BUG_REPORT] Plan {plan['plan_hash']} {decision} by {requester} (approval #{approval_id})")
        return f"Plan {plan['plan_hash']} for report #{plan['issue_id']} {decision}."

    def _run_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        denied = self._owner_only(permission_level)
        if denied:
            return denied
        issue_id = self._require_bug_id(bug_id)
        try:
            return self.worker.run(issue_id, requester)
        except WorkerError as exc:
            return f"Error: {exc}"

    def _cancel_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        denied = self._owner_only(permission_level)
        if denied:
            return denied
        issue_id = self._require_bug_id(bug_id)
        self.store.add_comment(issue_id, requester, "Cancel requested by owner.", kind="cancel")
        if self.store.request_cancel(issue_id):
            return f"Cancel requested for report #{issue_id}. The active run is marked cancel_requested."
        return f"Cancel note recorded for report #{issue_id}, but no run exists yet."

    def _artifacts_required(
        self,
        bug_id: Optional[int],
        run_id: Optional[int],
        requester: str,
        permission_level: str,
    ) -> str:
        if permission_level not in ("owner", "admin"):
            return "Permission denied: Only admins and owners can view workflow artifacts."
        issue_id = self._require_bug_id(bug_id)
        artifacts = self.store.list_artifacts(issue_id, run_id=run_id, limit=8)
        if not artifacts:
            return f"No artifacts found for report #{issue_id}."
        parts = []
        for artifact in artifacts:
            target = artifact.get("botbin_url") or artifact.get("path")
            parts.append(f"{artifact['kind']}: {target}")
        return f"Artifacts for report #{issue_id}: " + " | ".join(parts)

    def _status_required(self, bug_id: Optional[int], requester: str, permission_level: str) -> str:
        issue = self._get_visible_issue(self._require_bug_id(bug_id), requester, permission_level)
        latest_plan = self.store.get_latest_plan(int(issue["id"])) if issue.get("source") != "legacy_bug" else None
        latest_run = self.store.get_latest_run(int(issue["id"])) if issue.get("source") != "legacy_bug" else None
        parts = [f"Report #{issue['id']} status: {issue.get('status')} priority={issue.get('priority')} type={issue.get('type')}"]
        if latest_plan:
            parts.append(f"plan_hash={latest_plan['plan_hash']} automation_safe={latest_plan['automation_safe']}")
        if latest_run:
            parts.append(f"latest_run=#{latest_run['id']} status={latest_run['status']} branch={latest_run.get('branch_name') or '-'} pr={latest_run.get('pr_url') or '-'}")
        return " | ".join(parts)
