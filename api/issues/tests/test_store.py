import tempfile
import unittest
from pathlib import Path

from api.issues.store import IssueStore


class IssueStoreTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.store = IssueStore(Path(self.tmp.name) / "bugs.db")

    def tearDown(self):
        self.tmp.cleanup()

    def test_create_and_list_report_permissions(self):
        issue_id = self.store.create_issue("bug", "image generation fails when input is empty", "alice", "#chan")
        self.assertGreater(issue_id, 0)

        alice_reports = self.store.list_reports("alice", "normal", "open")
        bob_reports = self.store.list_reports("bob", "normal", "open")
        owner_reports = self.store.list_reports("owner", "owner", "open")

        self.assertEqual([item["id"] for item in alice_reports], [issue_id])
        self.assertEqual(bob_reports, [])
        self.assertEqual([item["id"] for item in owner_reports], [issue_id])

    def test_legacy_bug_can_be_imported_with_same_visible_id(self):
        with self.store._connect() as conn:
            conn.execute(
                """
                INSERT INTO bugs(reporter, channel, description, status, priority, created_at, updated_at)
                VALUES ('alice', '#chan', 'legacy bug still visible', 'open', 'normal', '2026-01-01', '2026-01-01')
                """
            )
            legacy_id = conn.execute("SELECT id FROM bugs").fetchone()[0]

        imported = self.store.ensure_issue(legacy_id)
        self.assertIsNotNone(imported)
        self.assertEqual(imported["id"], legacy_id)
        self.assertEqual(imported["source_bug_id"], legacy_id)

    def test_reporter_can_resolve_own_report(self):
        issue_id = self.store.create_issue("feature", "add retry handling for transient API failures", "alice", "#chan")
        self.assertTrue(self.store.resolve_report(issue_id, "alice", "done"))
        issue = self.store.get_issue(issue_id)
        self.assertEqual(issue["status"], "resolved")


if __name__ == "__main__":
    unittest.main()

