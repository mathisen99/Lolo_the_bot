import unittest

from api.issues.models import compute_plan_hash


class PlanHashTests(unittest.TestCase):
    def test_plan_hash_is_deterministic(self):
        plan_a = {
            "summary": "Fix bug",
            "issue_type": "bug",
            "requested_change": "Change behavior",
            "affected_paths": ["b.py", "a.py"],
            "risk_level": "low",
            "acceptance_criteria": ["works"],
            "test_plan": ["run tests"],
            "rollback_plan": "revert",
            "automation_safe": True,
        }
        plan_b = dict(plan_a)
        plan_b["affected_paths"] = ["a.py", "b.py"]
        self.assertEqual(compute_plan_hash(plan_a), compute_plan_hash(plan_b))

    def test_plan_hash_changes_for_material_change(self):
        plan = {
            "summary": "Fix bug",
            "issue_type": "bug",
            "requested_change": "Change behavior",
            "affected_paths": ["a.py"],
            "risk_level": "low",
            "acceptance_criteria": ["works"],
            "test_plan": ["run tests"],
            "rollback_plan": "revert",
            "automation_safe": True,
        }
        changed = dict(plan)
        changed["requested_change"] = "Change different behavior"
        self.assertNotEqual(compute_plan_hash(plan), compute_plan_hash(changed))


if __name__ == "__main__":
    unittest.main()

