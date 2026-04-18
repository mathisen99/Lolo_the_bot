import tempfile
import unittest
from pathlib import Path

from api.issues.artifacts import ArtifactManager
from api.issues.store import IssueStore
from api.tools.bug_report import BugReportTool


class FakePlanner:
    def plan(self, issue, comments):
        return {
            "summary": "Fix empty input handling",
            "issue_type": "bug",
            "requested_change": "Handle empty image prompt input without crashing.",
            "affected_paths": ["api/tools/gpt_image.py"],
            "risk_level": "low",
            "acceptance_criteria": ["Empty input returns a clear error"],
            "test_plan": ["Add focused unit test"],
            "rollback_plan": "Revert the tool change.",
            "automation_safe": True,
            "plan_text": "Plan text",
            "plan_hash": "abc123def4567890",
        }


class FakeWorker:
    def run(self, issue_id, requested_by):
        return f"ran {issue_id} for {requested_by}"


class BugReportPermissionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        db_path = Path(self.tmp.name) / "bugs.db"
        artifact_root = Path(self.tmp.name) / "artifacts"
        self.store = IssueStore(db_path)
        self.tool = BugReportTool(
            store=self.store,
            planner=FakePlanner(),
            worker=FakeWorker(),
            artifacts=ArtifactManager(self.store, root=artifact_root),
        )

    def tearDown(self):
        self.tmp.cleanup()

    def test_normal_user_cannot_plan_or_run(self):
        issue_id = self.store.create_issue("bug", "something broken in a repeatable way", "alice", "#chan")
        plan_result = self.tool.execute(
            action="plan",
            bug_id=issue_id,
            permission_level="normal",
            requesting_user="alice",
        )
        run_result = self.tool.execute(
            action="run",
            bug_id=issue_id,
            permission_level="admin",
            requesting_user="admin",
        )
        self.assertIn("Permission denied", plan_result)
        self.assertIn("Permission denied", run_result)

    def test_owner_can_plan_and_approve_single_pending_plan(self):
        issue_id = self.store.create_issue("bug", "something broken in a repeatable way", "alice", "#chan")
        plan_result = self.tool.execute(
            action="plan",
            bug_id=issue_id,
            permission_level="owner",
            requesting_user="owner",
        )
        self.assertIn("Plan ready", plan_result)

        approve_result = self.tool.execute(
            action="approve_plan",
            permission_level="owner",
            requesting_user="owner",
        )
        self.assertIn("approved", approve_result)

    def test_ambiguous_approval_is_rejected(self):
        first = self.store.create_issue("bug", "first broken behavior report", "alice", "#chan")
        second = self.store.create_issue("bug", "second broken behavior report", "alice", "#chan")
        self.tool.execute(action="plan", bug_id=first, permission_level="owner", requesting_user="owner")
        self.tool.execute(action="plan", bug_id=second, permission_level="owner", requesting_user="owner")

        result = self.tool.execute(action="approve_plan", permission_level="owner", requesting_user="owner")
        self.assertIn("ambiguous", result.lower())


if __name__ == "__main__":
    unittest.main()

