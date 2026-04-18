import subprocess
import tempfile
import unittest
from pathlib import Path

from api.issues.policy import PolicyChecker


class PolicyTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.repo = Path(self.tmp.name)
        subprocess.run(["git", "init"], cwd=self.repo, check=True, capture_output=True)
        subprocess.run(["git", "config", "user.email", "test@example.invalid"], cwd=self.repo, check=True)
        subprocess.run(["git", "config", "user.name", "Test"], cwd=self.repo, check=True)
        (self.repo / "README.md").write_text("initial\n", encoding="utf-8")
        subprocess.run(["git", "add", "README.md"], cwd=self.repo, check=True)
        subprocess.run(["git", "commit", "-m", "initial"], cwd=self.repo, check=True, capture_output=True)

    def tearDown(self):
        self.tmp.cleanup()

    def test_blocks_env_file(self):
        (self.repo / ".env").write_text("SECRET=value\n", encoding="utf-8")
        result = PolicyChecker().check(self.repo)
        self.assertFalse(result.ok)
        self.assertTrue(any("blocked path" in msg for msg in result.messages))

    def test_blocks_oversized_diff(self):
        (self.repo / "large.txt").write_text("\n".join(str(i) for i in range(30)), encoding="utf-8")
        result = PolicyChecker(max_diff_lines=10).check(self.repo)
        self.assertFalse(result.ok)
        self.assertTrue(any("diff too large" in msg for msg in result.messages))


if __name__ == "__main__":
    unittest.main()

