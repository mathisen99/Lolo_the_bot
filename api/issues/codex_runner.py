"""Write-enabled Codex runner used only after owner approval."""

from __future__ import annotations

import shutil
import subprocess
import time
from pathlib import Path
from typing import Callable, Dict, Optional


class CodexRunError(RuntimeError):
    pass


class CodexRunner:
    def __init__(self, codex_bin: str = "codex", timeout: int = 1800):
        self.codex_bin = codex_bin
        self.timeout = timeout

    def probe(self) -> None:
        if not shutil.which(self.codex_bin):
            raise CodexRunError(f"Codex CLI not found: {self.codex_bin}")

    def run(
        self,
        worktree: Path,
        prompt: str,
        final_message_path: Path,
        should_cancel: Optional[Callable[[], bool]] = None,
    ) -> Dict[str, object]:
        self.probe()
        final_message_path.parent.mkdir(parents=True, exist_ok=True)
        cmd = [
            self.codex_bin,
            "exec",
            "--cd",
            str(worktree),
            "--full-auto",
            "--json",
            "--output-last-message",
            str(final_message_path),
            "-",
        ]
        proc = subprocess.Popen(
            cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        deadline = time.monotonic() + self.timeout
        cancelled = False
        input_sent = False
        while True:
            try:
                remaining = max(0.1, min(1.0, deadline - time.monotonic()))
                stdout, stderr = proc.communicate(input=prompt if not input_sent else None, timeout=remaining)
                break
            except subprocess.TimeoutExpired:
                input_sent = True
                if should_cancel and should_cancel():
                    cancelled = True
                    proc.terminate()
                    try:
                        stdout, stderr = proc.communicate(timeout=10)
                    except subprocess.TimeoutExpired:
                        proc.kill()
                        stdout, stderr = proc.communicate()
                    break
                if time.monotonic() >= deadline:
                    proc.kill()
                    stdout, stderr = proc.communicate()
                    raise CodexRunError(f"Codex CLI timed out after {self.timeout}s")

        return {
            "returncode": -15 if cancelled else proc.returncode,
            "stdout": stdout,
            "stderr": stderr + ("\nCancelled by owner request." if cancelled else ""),
            "final_message": final_message_path.read_text(encoding="utf-8") if final_message_path.exists() else "",
            "cancelled": cancelled,
        }
