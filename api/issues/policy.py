"""Policy checks for AI-generated repository changes."""

from __future__ import annotations

import fnmatch
import os
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Optional


BLOCKED_PATTERNS = [
    ".env",
    ".env.*",
    "data/*.db",
    "data/**/*.db",
    "*.sqlite",
    "*.sqlite3",
    "*.key",
    "*.pem",
    "*.p12",
    "*.pfx",
    "config/bot.toml",
    "*.png",
    "*.jpg",
    "*.jpeg",
    "*.gif",
    "*.webp",
    "Lolo",
    "lolo",
    ".codex",
    ".codex/**",
]


@dataclass
class PolicyResult:
    ok: bool
    messages: List[str]
    changed_files: List[str]
    diff_lines: int

    def render(self) -> str:
        status = "PASS" if self.ok else "FAIL"
        lines = [
            f"Policy: {status}",
            f"Changed files: {len(self.changed_files)}",
            f"Diff lines: {self.diff_lines}",
        ]
        lines.extend(f"- {msg}" for msg in self.messages)
        if self.changed_files:
            lines.append("Files:")
            lines.extend(f"- {path}" for path in self.changed_files)
        return "\n".join(lines)


class PolicyChecker:
    def __init__(self, max_files: int = 20, max_diff_lines: int = 5000):
        self.max_files = max_files
        self.max_diff_lines = max_diff_lines

    def check(self, worktree: Path, planned_paths: Optional[Iterable[str]] = None) -> PolicyResult:
        messages: List[str] = []
        changed_files = self._changed_files(worktree)
        diff_lines, binary_files = self._diff_stats(worktree)

        if len(changed_files) > self.max_files:
            messages.append(f"too many changed files: {len(changed_files)} > {self.max_files}")
        if diff_lines > self.max_diff_lines:
            messages.append(f"diff too large: {diff_lines} > {self.max_diff_lines}")
        for path in changed_files:
            if self._is_blocked(path):
                messages.append(f"blocked path changed: {path}")
        for path in binary_files:
            messages.append(f"binary file changed: {path}")

        planned = {p.strip().rstrip("/") for p in (planned_paths or []) if p and p.strip()}
        if planned:
            for path in changed_files:
                if self._is_test_or_doc(path):
                    continue
                if not any(path == item or path.startswith(item + "/") for item in planned):
                    messages.append(f"changed path not in approved plan: {path}")

        return PolicyResult(
            ok=not messages,
            messages=messages or ["all policy checks passed"],
            changed_files=changed_files,
            diff_lines=diff_lines,
        )

    def _run(self, worktree: Path, args: list[str]) -> str:
        result = subprocess.run(args, cwd=str(worktree), capture_output=True, text=True, timeout=120)
        if result.returncode != 0:
            return ""
        return result.stdout

    def _changed_files(self, worktree: Path) -> List[str]:
        output = self._run(worktree, ["git", "status", "--porcelain", "--untracked-files=all"])
        files: List[str] = []
        for line in output.splitlines():
            if not line.strip():
                continue
            path = line[3:].strip()
            if " -> " in path:
                path = path.split(" -> ", 1)[1].strip()
            if self._is_runtime_metadata(path):
                continue
            files.append(path)
        return sorted(set(files))

    def _diff_stats(self, worktree: Path) -> tuple[int, List[str]]:
        tracked = self._run(worktree, ["git", "diff", "--numstat", "HEAD", "--"])
        untracked = self._run(worktree, ["git", "ls-files", "--others", "--exclude-standard"])
        diff_lines = 0
        binary: List[str] = []
        for line in tracked.splitlines():
            parts = line.split("\t")
            if len(parts) < 3:
                continue
            added, deleted, path = parts[0], parts[1], parts[2]
            if added == "-" or deleted == "-":
                binary.append(path)
                continue
            diff_lines += int(added) + int(deleted)
        for path in untracked.splitlines():
            if self._is_runtime_metadata(path):
                continue
            file_path = worktree / path
            if file_path.is_file():
                if self._looks_binary(file_path):
                    binary.append(path)
                else:
                    try:
                        diff_lines += len(file_path.read_text(encoding="utf-8", errors="replace").splitlines())
                    except OSError:
                        pass
        return diff_lines, binary

    def _is_blocked(self, path: str) -> bool:
        clean = path.strip().lstrip("/")
        if clean.startswith("../") or "/../" in clean:
            return True
        return any(fnmatch.fnmatch(clean, pattern) for pattern in BLOCKED_PATTERNS)

    def _is_test_or_doc(self, path: str) -> bool:
        parts = set(Path(path).parts)
        base = os.path.basename(path)
        return (
            path.endswith("_test.go")
            or base.startswith("test_")
            or base.endswith("_test.py")
            or "tests" in parts
            or path.endswith(".md")
        )

    def _is_runtime_metadata(self, path: str) -> bool:
        clean = path.strip().lstrip("/")
        return clean == ".codex" or clean.startswith(".codex/")

    def _looks_binary(self, path: Path) -> bool:
        try:
            with path.open("rb") as f:
                chunk = f.read(4096)
        except OSError:
            return False
        return b"\0" in chunk
