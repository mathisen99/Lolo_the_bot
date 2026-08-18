import unittest

from api.ai.client import AIClient


class AIClientTests(unittest.TestCase):
    def test_raw_botbin_url_gets_formatted_view_companion(self):
        result = AIClient._ensure_botbin_formatted_urls(
            "Code: https://botbin.net/abc123"
        )

        self.assertEqual(
            result,
            "Code: https://botbin.net/abc123 | "
            "Formatted: https://botbin.net/paste/abc123",
        )

    def test_existing_formatted_botbin_url_is_not_duplicated(self):
        text = (
            "Code: https://botbin.net/abc123 | "
            "Formatted: https://botbin.net/paste/abc123"
        )

        self.assertEqual(AIClient._ensure_botbin_formatted_urls(text), text)

    def test_obvious_code_request_bypasses_responses_api(self):
        client = AIClient.__new__(AIClient)
        client.tools = {"codex_code": RecordingCodexTool("Codex answer")}
        client.trace_store = None

        events = list(client.generate_response_with_context_stream(
            user_message="write me a Python function",
            nick="alice",
            channel="#code",
            conversation_history=[],
            permission_level="normal",
            command_prefix="!",
            request_id="direct-codex",
        ))

        self.assertEqual(events, [{"status": "success", "message": "Codex answer"}])
        self.assertEqual(
            client.tools["codex_code"].calls,
            [{"question": "write me a Python function", "context": ""}],
        )

    def test_tool_definitions_require_public_audit_reason(self):
        client = AIClient.__new__(AIClient)
        client.tools = {
            "normal": DefinitionTool("normal"),
            "show_execution_steps": DefinitionTool("show_execution_steps"),
        }

        normal, trace_lookup = client._get_tool_definitions()

        self.assertIn("audit_reason", normal["parameters"]["properties"])
        self.assertIn("audit_reason", normal["parameters"]["required"])
        self.assertNotIn("audit_reason", trace_lookup["parameters"]["properties"])

    def test_trace_sanitizer_preserves_evidence_and_redacts_secrets(self):
        rendered = AIClient._format_trace_value({
            "url": "https://example.com/source?id=42",
            "question": "Check whether the screenshot was edited",
            "api_key": "sk-this-should-not-appear",
            "image": "data:image/png;base64,QUJDREVGRw==",
            "_current_channel": "#internal",
        })

        self.assertIn("https://example.com/source?id=42", rendered)
        self.assertIn("Check whether the screenshot was edited", rendered)
        self.assertIn("[redacted]", rendered)
        self.assertIn("[embedded image data omitted]", rendered)
        self.assertNotIn("sk-this-should-not-appear", rendered)
        self.assertNotIn("_current_channel", rendered)

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


class RecordingCodexTool:
    def __init__(self, result):
        self.result = result
        self.calls = []

    @staticmethod
    def should_route_directly(message):
        return True

    def execute(self, **kwargs):
        self.calls.append(kwargs)
        return self.result


class DefinitionTool:
    def __init__(self, name):
        self.name = name

    def get_definition(self):
        return {
            "type": "function",
            "name": self.name,
            "parameters": {
                "type": "object",
                "properties": {},
                "required": [],
                "additionalProperties": False,
            },
        }


if __name__ == "__main__":
    unittest.main()
