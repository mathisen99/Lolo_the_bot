import tempfile
import unittest
import sqlite3
from pathlib import Path

from api.execution_trace import ExecutionTraceStore
from api.tools.execution_steps import ExecutionStepsTool


class ExecutionTraceTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.store = ExecutionTraceStore(Path(self.temp_dir.name) / "traces.db")

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_latest_trace_is_scoped_and_excludes_current_request(self):
        self.store.start_run("old", "Alice", "libera", "#chat", "Check this claim")
        self.store.append_step(
            "old",
            "Searched the web",
            tool_name="web_search",
            details='{"query": "example claim"}',
            result_summary="Source: https://example.com/evidence",
        )
        self.store.append_step("old", "Read the supplied webpage", "failed")
        self.store.finish_run("old", "success", "The claim was not verified.")
        self.store.start_run("current", "Alice", "libera", "#chat")

        trace = self.store.latest_trace(
            "alice", "LIBERA", "#CHAT", exclude_request_id="current"
        )

        self.assertEqual(trace["request_id"], "old")
        self.assertEqual(trace["objective"], "Check this claim")
        self.assertEqual(trace["final_answer"], "The claim was not verified.")
        self.assertEqual(len(trace["steps"]), 2)
        self.assertEqual(trace["steps"][0]["tool_name"], "web_search")
        self.assertIn("example claim", trace["steps"][0]["details"])
        self.assertIn("https://example.com/evidence", trace["steps"][0]["result_summary"])

    def test_trace_cannot_be_read_by_another_user(self):
        self.store.start_run("alice-run", "alice", "libera", "#chat")
        self.store.append_step("alice-run", "Searched the web")
        self.store.finish_run("alice-run", "success")

        self.assertIsNone(self.store.latest_trace("bob", "libera", "#chat"))

    def test_tool_returns_sanitized_numbered_steps(self):
        self.store.start_run(
            "previous", "alice", "libera", "#chat", "Calculate the total"
        )
        self.store.append_step(
            "previous",
            "Ran a calculation or data analysis",
            tool_name="python_exec",
            details="print(sum([10, 20]))",
            result_summary="Output: 30",
        )
        self.store.finish_run("previous", "success", "The total is 30.")
        self.store.start_run("current", "alice", "libera", "#chat")
        tool = ExecutionStepsTool(self.store)

        result = tool.execute(
            _requesting_user="alice",
            _current_network="libera",
            _current_channel="#chat",
            _current_request_id="current",
        )

        self.assertIn("not private chain-of-thought", result)
        self.assertIn("Original request:\nCalculate the total", result)
        self.assertIn("1. Ran a calculation or data analysis", result)
        self.assertIn("[python_exec]", result)
        self.assertIn("print(sum([10, 20]))", result)
        self.assertIn("Observed result/evidence:\nOutput: 30", result)
        self.assertIn("Answer produced from the above evidence:\nThe total is 30.", result)

        self.store.finish_run("current", "success")
        self.store.start_run("second-lookup", "alice", "libera", "#chat")
        repeated = tool.execute(
            _requesting_user="alice",
            _current_network="libera",
            _current_channel="#chat",
            _current_request_id="second-lookup",
        )
        self.assertIn("1. Ran a calculation or data analysis", repeated)

    def test_existing_small_schema_is_migrated(self):
        db_path = Path(self.temp_dir.name) / "legacy.db"
        with sqlite3.connect(db_path) as connection:
            connection.executescript(
                """
                CREATE TABLE execution_runs (
                    request_id TEXT PRIMARY KEY, nick TEXT NOT NULL,
                    network TEXT NOT NULL, channel TEXT NOT NULL,
                    started_at TEXT NOT NULL, finished_at TEXT, status TEXT NOT NULL
                );
                CREATE TABLE execution_steps (
                    id INTEGER PRIMARY KEY AUTOINCREMENT, request_id TEXT NOT NULL,
                    position INTEGER NOT NULL, summary TEXT NOT NULL,
                    outcome TEXT NOT NULL, created_at TEXT NOT NULL,
                    UNIQUE(request_id, position)
                );
                """
            )

        migrated = ExecutionTraceStore(db_path)
        migrated.start_run("new", "alice", "libera", "#chat", "Investigate")
        migrated.append_step(
            "new", "Read a page", tool_name="fetch_url", details="https://example.com"
        )
        migrated.finish_run("new", "success", "Done")

        trace = migrated.latest_trace("alice", "libera", "#chat")
        self.assertEqual(trace["objective"], "Investigate")
        self.assertEqual(trace["steps"][0]["tool_name"], "fetch_url")


if __name__ == "__main__":
    unittest.main()
