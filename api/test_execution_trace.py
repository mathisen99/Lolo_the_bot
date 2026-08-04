import tempfile
import unittest
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
        self.store.start_run("old", "Alice", "libera", "#chat")
        self.store.append_step("old", "Searched the web")
        self.store.append_step("old", "Read the supplied webpage", "failed")
        self.store.finish_run("old", "success")
        self.store.start_run("current", "Alice", "libera", "#chat")

        trace = self.store.latest_trace(
            "alice", "LIBERA", "#CHAT", exclude_request_id="current"
        )

        self.assertEqual(trace["request_id"], "old")
        self.assertEqual(
            trace["steps"],
            [
                {"summary": "Searched the web", "outcome": "completed"},
                {"summary": "Read the supplied webpage", "outcome": "failed"},
            ],
        )

    def test_trace_cannot_be_read_by_another_user(self):
        self.store.start_run("alice-run", "alice", "libera", "#chat")
        self.store.append_step("alice-run", "Searched the web")
        self.store.finish_run("alice-run", "success")

        self.assertIsNone(self.store.latest_trace("bob", "libera", "#chat"))

    def test_tool_returns_sanitized_numbered_steps(self):
        self.store.start_run("previous", "alice", "libera", "#chat")
        self.store.append_step("previous", "Ran a calculation or data analysis")
        self.store.finish_run("previous", "success")
        self.store.start_run("current", "alice", "libera", "#chat")
        tool = ExecutionStepsTool(self.store)

        result = tool.execute(
            _requesting_user="alice",
            _current_network="libera",
            _current_channel="#chat",
            _current_request_id="current",
        )

        self.assertIn("not private chain-of-thought", result)
        self.assertIn("1. Ran a calculation or data analysis", result)

        self.store.finish_run("current", "success")
        self.store.start_run("second-lookup", "alice", "libera", "#chat")
        repeated = tool.execute(
            _requesting_user="alice",
            _current_network="libera",
            _current_channel="#chat",
            _current_request_id="second-lookup",
        )
        self.assertIn("1. Ran a calculation or data analysis", repeated)


if __name__ == "__main__":
    unittest.main()
