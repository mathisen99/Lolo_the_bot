import unittest

from api.ai.usage_tracker import calculate_multimodal_cost, extract_usage_from_image_result


class UsageTrackerTests(unittest.TestCase):
    def test_extract_usage_from_image_result(self):
        result = {
            "usage": {
                "input_tokens": 120,
                "output_tokens": 345,
                "input_tokens_details": {
                    "cached_tokens": 20,
                    "text_tokens": 80,
                    "image_tokens": 40,
                },
            }
        }

        usage = extract_usage_from_image_result(result)

        self.assertEqual(
            usage,
            {
                "input_tokens": 120,
                "cached_tokens": 20,
                "output_tokens": 345,
            },
        )

    def test_calculate_multimodal_cost_for_gpt_image_2(self):
        usage = {
            "input_tokens": 3000,
            "output_tokens": 7000,
            "input_tokens_details": {
                "text_tokens": 1000,
                "image_tokens": 2000,
                "cached_text_tokens": 100,
                "cached_image_tokens": 200,
            },
            "output_tokens_details": {
                "text_tokens": 300,
                "image_tokens": 6700,
            },
        }

        cost = calculate_multimodal_cost("gpt-image-2", usage)

        expected = (
            (900 / 1_000_000) * 5.00 +
            (100 / 1_000_000) * 1.25 +
            (300 / 1_000_000) * 10.00 +
            (1800 / 1_000_000) * 8.00 +
            (200 / 1_000_000) * 2.00 +
            (6700 / 1_000_000) * 30.00
        )
        self.assertIsNotNone(cost)
        self.assertAlmostEqual(cost, expected, places=10)

    def test_calculate_multimodal_cost_splits_aggregate_cached_tokens(self):
        usage = {
            "input_tokens": 400,
            "output_tokens": 100,
            "input_tokens_details": {
                "text_tokens": 100,
                "image_tokens": 300,
                "cached_tokens": 40,
            },
            "output_tokens_details": {
                "image_tokens": 100,
            },
        }

        cost = calculate_multimodal_cost("gpt-image-2", usage)

        expected = (
            (90 / 1_000_000) * 5.00 +
            (10 / 1_000_000) * 1.25 +
            (270 / 1_000_000) * 8.00 +
            (30 / 1_000_000) * 2.00 +
            (100 / 1_000_000) * 30.00
        )
        self.assertIsNotNone(cost)
        self.assertAlmostEqual(cost, expected, places=10)


if __name__ == "__main__":
    unittest.main()
