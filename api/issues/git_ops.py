"""Git and GitHub operations for isolated issue worktrees."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path
from typing import Optional

from .models import slugify


class GitError(RuntimeError):
    pass


class GitOps:
    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = Path(repo_root) if repo_root else Path(__file__).resolve().parents[2]

    def _run(self, args: list[str], cwd: Optional[Path] = None, timeout: int = 120) -> str:
        result = subprocess.run(
            args,
            cwd=str(cwd or self.repo_root),
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        if result.returncode != 0:
            raise GitError(result.stderr.strip() or result.stdout.strip() or f"{args[0]} failed")
        return result.stdout.strip()

    def ensure_clean_tracked_state(self) -> None:
        status = self._run(["git", "status", "--porcelain", "--untracked-files=no"])
        if status.strip():
            raise GitError("tracked files are dirty in the main repo; refusing automation run")

    def fetch_origin_main(self) -> str:
        self._run(["git", "fetch", "origin", "main"], timeout=300)
        return self._run(["git", "rev-parse", "origin/main"])

    def create_worktree(self, issue_id: int, title: str, base_sha: str, root: Path, run_id: Optional[int] = None) -> tuple[str, Path]:
        root.mkdir(parents=True, exist_ok=True)
        suffix = f"run-{run_id}" if run_id is not None else slugify(title)
        branch = f"lolo/issue-{issue_id}-{slugify(title)}-{suffix}"
        worktree = root / f"issue-{issue_id}-{slugify(title)}-{suffix}"
        if worktree.exists():
            raise GitError(f"worktree already exists: {worktree}")
        self._run(["git", "worktree", "add", "-b", branch, str(worktree), base_sha], timeout=300)
        return branch, worktree

    def diff(self, worktree: Path) -> str:
        diff = self._run(["git", "diff", "--", "."], cwd=worktree, timeout=120)
        untracked = self._run(["git", "ls-files", "--others", "--exclude-standard"], cwd=worktree)
        extra: list[str] = []
        for path in untracked.splitlines():
            file_path = worktree / path
            if not file_path.is_file():
                continue
            try:
                content = file_path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            extra.append(f"--- /dev/null\n+++ b/{path}\n@@ new file @@\n{content}")
        if extra:
            return "\n".join(part for part in [diff, *extra] if part)
        return diff

    def changed_files(self, worktree: Path) -> list[str]:
        output = self._run(["git", "status", "--porcelain", "--untracked-files=all"], cwd=worktree)
        files: list[str] = []
        for line in output.splitlines():
            if not line.strip():
                continue
            path = line[3:].strip()
            if " -> " in path:
                path = path.split(" -> ", 1)[1].strip()
            files.append(path)
        return files

    def cleanup_runtime_metadata(self, worktree: Path) -> None:
        """Remove local agent metadata that must never be committed."""
        codex_path = worktree / ".codex"
        if codex_path.is_dir():
            shutil.rmtree(codex_path)
        elif codex_path.exists():
            codex_path.unlink()

    def commit_all(self, worktree: Path, message: str) -> str:
        self._run(["git", "add", "--all"], cwd=worktree)
        self._run(["git", "commit", "-m", message], cwd=worktree, timeout=300)
        return self._run(["git", "rev-parse", "HEAD"], cwd=worktree)

    def push(self, worktree: Path, branch: str) -> None:
        self._run(["git", "push", "origin", f"HEAD:{branch}"], cwd=worktree, timeout=300)

    def probe_gh(self) -> None:
        if not shutil.which("gh"):
            raise GitError("gh CLI not found")
        self._run(["gh", "auth", "status"], timeout=30)
        self._run(["git", "remote", "get-url", "origin"], timeout=30)
        self._run(["gh", "repo", "view", "--json", "nameWithOwner"], timeout=60)

    def create_draft_pr(self, worktree: Path, branch: str, title: str, body_file: Path) -> str:
        self.probe_gh()
        output = self._run(
            [
                "gh",
                "pr",
                "create",
                "--draft",
                "--base",
                "main",
                "--head",
                branch,
                "--title",
                title,
                "--body-file",
                str(body_file),
            ],
            cwd=worktree,
            timeout=120,
        )
        return output.strip().splitlines()[-1] if output.strip() else ""
