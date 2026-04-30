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


if __name__ == "__main__":
    unittest.main()
