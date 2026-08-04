import unittest

from api.ai.client import AIClient


class AIClientTests(unittest.TestCase):
    def test_build_input_image_content_preserves_detail(self):
        content = AIClient._build_input_image_content(
            "data:image/png;base64,abc",
            "low",
        )

        self.assertEqual(
            content,
            {
                "type": "input_image",
                "image_url": "data:image/png;base64,abc",
                "detail": "low",
            },
        )

    def test_build_input_image_content_omits_empty_detail(self):
        content = AIClient._build_input_image_content(
            "data:image/png;base64,abc",
            None,
        )

        self.assertEqual(
            content,
            {
                "type": "input_image",
                "image_url": "data:image/png;base64,abc",
            },
        )

    def test_raw_json_object_is_always_pasted(self):
        client, paste = self._client_with_paste()
        content = '{\n  "command": "whois",\n  "args": ["alice"]\n}'

        result = client._paste_json_for_irc(content, "test-json")

        self.assertEqual(result, "JSON: https://botbin.test/json")
        self.assertEqual(
            paste.calls,
            [{
                "content": content,
                "filename": "response.json",
                "retention": "1week",
            }],
        )

    def test_json_fence_in_explanation_is_replaced_with_paste(self):
        client, paste = self._client_with_paste()
        response = "Use this payload:\n```json\n{\"enabled\": true}\n```\nThen submit it."

        result = client._paste_json_for_irc(response, "test-json-fence")

        self.assertEqual(
            result,
            "Use this payload:\nJSON: https://botbin.test/json\nThen submit it.",
        )
        self.assertEqual(paste.calls[0]["content"], '{"enabled": true}')

    def test_unlabelled_json_fence_is_pasted(self):
        client, paste = self._client_with_paste()

        result = client._paste_json_for_irc(
            "```\n[1, 2, 3]\n```",
            "test-json-fence",
        )

        self.assertEqual(result, "JSON: https://botbin.test/json")
        self.assertEqual(paste.calls[0]["content"], "[1, 2, 3]")

    def test_json_scalars_and_brace_heavy_prose_remain_inline(self):
        client, paste = self._client_with_paste()

        self.assertEqual(client._paste_json_for_irc("true", "test-scalar"), "true")
        self.assertEqual(
            client._paste_json_for_irc("Use {name} in the template.", "test-prose"),
            "Use {name} in the template.",
        )
        self.assertEqual(paste.calls, [])

    def test_json_is_not_sent_inline_when_paste_fails(self):
        client, _ = self._client_with_paste(result="Error: service unavailable")
        content = '{"secret": "would-have-been-inline"}'

        result = client._paste_json_for_irc(content, "test-json-failure")

        self.assertEqual(
            result,
            "I couldn't upload the JSON response to BotBin. Please try again.",
        )
        self.assertNotIn("would-have-been-inline", result)

    @staticmethod
    def _client_with_paste(result="https://botbin.test/json"):
        client = AIClient.__new__(AIClient)
        paste = RecordingPasteTool(result)
        client.tools = {"create_paste": paste}
        return client, paste


class RecordingPasteTool:
    def __init__(self, result):
        self.result = result
        self.calls = []

    def execute(self, **kwargs):
        self.calls.append(kwargs)
        return self.result


if __name__ == "__main__":
    unittest.main()
