import io
import unittest

from PIL import Image

from api.tools.gpt_image import GPTImageTool


class GPTImageToolTests(unittest.TestCase):
    def setUp(self):
        self.tool = GPTImageTool()
        self.tool.api_key = "test-key"

    def test_validate_size_accepts_valid_custom_resolution(self):
        self.assertIsNone(self.tool._validate_size("2048x1152"))

    def test_validate_size_rejects_invalid_resolution(self):
        error = self.tool._validate_size("1000x1000")
        self.assertIsNotNone(error)
        self.assertIn("Invalid size for gpt-image-2", error)

    def test_find_best_target_size_preserves_valid_size(self):
        size_str, size = self.tool._find_best_target_size(1024, 1536)
        self.assertEqual(size_str, "1024x1536")
        self.assertEqual(size, (1024, 1536))

    def test_find_best_target_size_rounds_non_multiple_dimensions(self):
        size_str, size = self.tool._find_best_target_size(1000, 1000)
        self.assertEqual(size_str, "1008x1008")
        self.assertEqual(size, (1008, 1008))

    def test_find_best_target_size_scales_oversized_images(self):
        _, size = self.tool._find_best_target_size(5000, 3000)
        self.assertTrue(self.tool._is_valid_api_size(*size))
        self.assertLessEqual(size[0], self.tool.MAX_EDGE)
        self.assertLessEqual(size[1], self.tool.MAX_EDGE)

    def test_prepare_mask_bytes_adds_alpha_and_matches_target_size(self):
        mask = Image.new("L", (64, 64), color=255)
        source = io.BytesIO()
        mask.save(source, format="PNG")

        result = self.tool._prepare_mask_bytes(
            source.getvalue(),
            first_image_size=(128, 128),
            target_size=(144, 160),
        )

        prepared = Image.open(io.BytesIO(result))
        self.assertEqual(prepared.mode, "RGBA")
        self.assertEqual(prepared.size, (144, 160))
        self.assertEqual(prepared.getpixel((0, 0))[3], 0)
        self.assertGreater(prepared.getpixel((72, 80))[3], 0)

    def test_execute_rejects_transparent_backgrounds(self):
        result = self.tool.execute(prompt="test prompt", background="transparent")
        self.assertEqual(result, "Error: gpt-image-2 does not support transparent backgrounds")

    def test_execute_requires_input_image_for_mask(self):
        result = self.tool.execute(prompt="test prompt", mask_url="https://example.com/mask.png")
        self.assertEqual(result, "Error: mask_url requires at least one input image")


if __name__ == "__main__":
    unittest.main()
