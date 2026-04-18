"""Verification command runner for issue worktrees."""

from __future__ import annotations

import shlex
import subprocess
import sys
from pathlib import Path
from typing import Iterable, List, Tuple


class Verifier:
    def __init__(self, timeout: int = 600):
        self.timeout = timeout

    def verify(self, worktree: Path, changed_files: Iterable[str]) -> Tuple[bool, str]:
        commands: List[List[str]] = [
            ["go", "test", "./..."],
            ["go", "build", "-o", "/tmp/lolo-issue-build", "./cmd/bot"],
        ]
        py_files = [str(worktree / path) for path in changed_files if path.endswith(".py")]
        if py_files:
            commands.append([sys.executable, "-m", "py_compile", *py_files])

        all_ok = True
        sections: List[str] = []
        for cmd in commands:
            ok, output = self._run(worktree, cmd)
            all_ok = all_ok and ok
            sections.append(f"$ {shlex.join(cmd)}\n{output.strip() or '(no output)'}\nexit_ok={ok}")
        return all_ok, "\n\n".join(sections)

    def _run(self, worktree: Path, cmd: List[str]) -> Tuple[bool, str]:
        try:
            result = subprocess.run(
                cmd,
                cwd=str(worktree),
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
        except FileNotFoundError as exc:
            return False, f"command not found: {cmd[0]} ({exc})"
        except subprocess.TimeoutExpired as exc:
            return False, f"timed out after {self.timeout}s\n{exc.stdout or ''}\n{exc.stderr or ''}"
        output = "\n".join(part for part in (result.stdout, result.stderr) if part)
        return result.returncode == 0, output
