"""
Log analyzer tool.

Reads bot.log and api.log from data/ and returns recent entries
for the AI to analyze errors, events, and operational status.
"""

import os
import re
from pathlib import Path
from typing import Any, Dict
from .base import Tool
from api.utils.output import log_info, log_warning


# Strip ANSI color codes from log lines
ANSI_RE = re.compile(r'\x1b\[[0-9;]*m')

BOT_LOG = Path("data/bot.log")
API_LOG = Path("data/api.log")
MAX_LINES = 200  # Safety cap per file


class LogAnalyzerTool(Tool):
    """Read and analyze bot/API log files."""

    @property
    def name(self) -> str:
        return "log_analyzer"

    def get_definition(self) -> Dict[str, Any]:
        return {
            "type": "function",
            "name": "log_analyzer",
            "description": (
                "Read recent log output from the bot (data/bot.log) and/or the Python API (data/api.log). "
                "Use this to diagnose errors, check what happened recently, review startup issues, "
                "or give the user a status report. Returns the last N lines (default 50) from the requested log. "
                "You can also filter lines by a keyword to narrow results."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "source": {
                        "type": "string",
                        "enum": ["bot", "api", "both"],
                        "description": "Which log to read: 'bot' for Go bot, 'api' for Python API, 'both' for both."
                    },
                    "lines": {
                        "type": "integer",
                        "description": "Number of recent lines to return (default 50, max 200)."
                    },
                    "filter": {
                        "type": "string",
                        "description": "Optional keyword filter — only return lines containing this text (case-insensitive)."
                    }
                },
                "required": ["source"],
                "additionalProperties": False
            }
        }

    def execute(
        self,
        source: str = "both",
        lines: int = 50,
        filter: str = "",
        permission_level: str = "normal",
        **kwargs
    ) -> str:
        # Owner-only tool
        if permission_level not in ("owner", "admin"):
            return "Error: Log analysis is restricted to owner and admin users."

        lines = min(max(lines, 1), MAX_LINES)
        results = []

        sources = []
        if source in ("bot", "both"):
            sources.append(("Bot (Go)", BOT_LOG))
        if source in ("api", "both"):
            sources.append(("API (Python)", API_LOG))

        for label, path in sources:
            if not path.exists():
                results.append(f"=== {label} ===\nLog file not found: {path}")
                continue

            try:
                with open(path, "r", errors="replace") as f:
                    all_lines = f.readlines()

                # Strip ANSI codes
                all_lines = [ANSI_RE.sub("", line).rstrip() for line in all_lines]

                # Apply keyword filter
                if filter:
                    all_lines = [l for l in all_lines if filter.lower() in l.lower()]

                # Take last N lines
                tail = all_lines[-lines:]

                log_info(f"[log_analyzer] Read {len(tail)} lines from {path} (total {len(all_lines)} after filter)")

                if not tail:
                    results.append(f"=== {label} ===\nNo matching lines found.")
                else:
                    results.append(f"=== {label} ({len(tail)} lines) ===\n" + "\n".join(tail))

            except Exception as e:
                log_warning(f"[log_analyzer] Error reading {path}: {e}")
                results.append(f"=== {label} ===\nError reading log: {e}")

        return "\n\n".join(results)
