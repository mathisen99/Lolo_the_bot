import unittest
from pathlib import Path

import tomli

from api.tools.report_status import ReportStatusTool


class ProgressPolicyPromptTests(unittest.TestCase):
    def test_prompt_limits_public_progress_and_supports_step_retrieval(self):
        config_path = Path(__file__).parent / "config" / "ai_settings.toml"
        with config_path.open("rb") as config_file:
            prompt = tomli.load(config_file)["system_prompt"]["text"]

        self.assertIn("QUICK or LONG_MULTI_STEP", prompt)
        self.assertIn("call report_status exactly ONCE", prompt)
        self.assertIn("Never call report_status more than once", prompt)
        self.assertIn("explains WHY this request will take time", prompt)
        self.assertIn("WHAT concrete checks or actions", prompt)
        self.assertIn("Never use a generic acknowledgement", prompt)
        self.assertIn("call show_execution_steps", prompt)
        self.assertNotIn("MUST call report_status between them", prompt)

    def test_report_status_tool_requires_request_specific_content(self):
        definition = ReportStatusTool().get_definition()
        tool_description = definition["description"]
        message_description = definition["parameters"]["properties"]["status_message"]["description"]

        self.assertIn("request-specific", tool_description)
        self.assertIn("why this request will take time", tool_description)
        self.assertIn("what concrete checks or actions", tool_description)
        self.assertIn("Never send generic or reusable boilerplate", tool_description)
        self.assertIn("tied to the request", message_description)


if __name__ == "__main__":
    unittest.main()
