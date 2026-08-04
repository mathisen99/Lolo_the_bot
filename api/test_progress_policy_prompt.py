import unittest
from pathlib import Path

import tomli


class ProgressPolicyPromptTests(unittest.TestCase):
    def test_prompt_limits_public_progress_and_supports_step_retrieval(self):
        config_path = Path(__file__).parent / "config" / "ai_settings.toml"
        with config_path.open("rb") as config_file:
            prompt = tomli.load(config_file)["system_prompt"]["text"]

        self.assertIn("QUICK or LONG_MULTI_STEP", prompt)
        self.assertIn("call report_status exactly ONCE", prompt)
        self.assertIn("Never call report_status more than once", prompt)
        self.assertIn("call show_execution_steps", prompt)
        self.assertNotIn("MUST call report_status between them", prompt)


if __name__ == "__main__":
    unittest.main()
