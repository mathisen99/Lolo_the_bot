import unittest
from pathlib import Path

import tomli

from api.tools.chat_history import ChatHistoryTool


class ContextRecallPromptTests(unittest.TestCase):
    def test_system_prompt_requires_contextual_search_before_negative_answer(self):
        config_path = Path(__file__).parent / "config" / "ai_settings.toml"
        with config_path.open("rb") as config_file:
            prompt = tomli.load(config_file)["system_prompt"]["text"]

        self.assertIn("Interpret every message as part of a conversation", prompt)
        self.assertIn("Separate meaning from wording", prompt)
        self.assertIn("Build history searches from the inferred intent", prompt)
        self.assertIn("start with semantic=true", prompt)
        self.assertIn(
            "Never conclude that something was not mentioned solely because one exact keyword search returned no match",
            prompt,
        )
        self.assertIn("Resolve pronouns, callbacks, implied requests, and follow-ups", prompt)

    def test_chat_history_tool_explains_that_keyword_misses_are_inconclusive(self):
        description = ChatHistoryTool().get_definition()["description"]

        self.assertIn("may be a paraphrase rather than a literal quote", description)
        self.assertIn("Infer the intended meaning from the conversation", description)
        self.assertIn("prefer semantic=true", description)
        self.assertIn("A failed exact keyword search proves only", description)


if __name__ == "__main__":
    unittest.main()
