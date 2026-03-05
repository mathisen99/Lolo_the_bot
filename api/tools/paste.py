"""
Paste tool implementation.

Provides text/code pasting to botbin.net for content that doesn't work well on IRC.
"""

import os
import tempfile
import requests
from typing import Any, Dict, Optional, List, Tuple
from .base import Tool


class PasteTool(Tool):
    """Paste tool using botbin.net API."""
    
    # Map expiry to botbin retention format
    EXPIRY_MAP = {
        "1day": "24h",
        "1week": "168h",
        "1month": "720h"
    }
    
    VALID_EXPIRIES = {"1day", "1week", "1month"}

    # Map common markdown language tags to file extensions
    LANGUAGE_EXTENSIONS = {
        "python": "py",
        "py": "py",
        "javascript": "js",
        "js": "js",
        "typescript": "ts",
        "ts": "ts",
        "c": "c",
        "cpp": "cpp",
        "c++": "cpp",
        "cc": "cpp",
        "cxx": "cpp",
        "csharp": "cs",
        "c#": "cs",
        "cs": "cs",
        "go": "go",
        "golang": "go",
        "rust": "rs",
        "rs": "rs",
        "java": "java",
        "kotlin": "kt",
        "kt": "kt",
        "swift": "swift",
        "php": "php",
        "ruby": "rb",
        "rb": "rb",
        "bash": "sh",
        "sh": "sh",
        "shell": "sh",
        "zsh": "sh",
        "powershell": "ps1",
        "ps1": "ps1",
        "sql": "sql",
        "html": "html",
        "css": "css",
        "scss": "scss",
        "sass": "sass",
        "json": "json",
        "yaml": "yml",
        "yml": "yml",
        "toml": "toml",
        "xml": "xml",
        "lua": "lua",
        "perl": "pl",
        "r": "r",
    }
    
    def __init__(self):
        """Initialize paste tool."""
        self.api_url = "https://botbin.net/upload"
        self.api_key = os.environ.get("BOTBIN_API_KEY")
    
    @property
    def name(self) -> str:
        return "create_paste"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "create_paste",
            "description": "Create a paste on botbin.net for content that doesn't work well on IRC (code, long text, formatted content). Use this when your response would exceed 3 IRC messages or contains code/formatted text that needs proper display. Returns a short URL to the paste.",
            "parameters": {
                "type": "object",
                "properties": {
                    "content": {
                        "type": "string",
                        "description": "The text or code content to paste"
                    },
                    "filename": {
                        "type": "string",
                        "description": "Filename for the paste with extension (e.g., 'example.py', 'code.js', 'notes.txt'). Extension determines content type display. If omitted or generic (.txt), extension may be auto-inferred for fenced code."
                    },
                    "retention": {
                        "type": "string",
                        "enum": ["1day", "1week", "1month"],
                        "description": "How long to keep the paste. Default: 1week"
                    }
                },
                "required": ["content"],
                "additionalProperties": False
            }
        }

    def _normalize_language_tag(self, tag: str) -> str:
        """Normalize a markdown fence language tag."""
        if not tag:
            return ""
        normalized = tag.strip().lower()
        normalized = normalized.strip("{}").lstrip(".")
        if normalized.startswith("language-"):
            normalized = normalized[len("language-"):]
        normalized = normalized.split()[0]
        return normalized

    def _language_to_extension(self, language: str) -> Optional[str]:
        """Map a language hint to extension."""
        normalized = self._normalize_language_tag(language)
        if not normalized:
            return None
        return self.LANGUAGE_EXTENSIONS.get(normalized)

    def _extract_fenced_blocks_with_context(self, text: str) -> Tuple[List[Tuple[str, str]], bool]:
        """
        Extract triple-backtick fenced code blocks.

        Returns:
            (blocks, has_non_code_text)
        """
        blocks: List[Tuple[str, str]] = []
        in_block = False
        language = ""
        current_lines: List[str] = []
        has_non_code_text = False

        for line in text.splitlines():
            stripped = line.strip()

            if not in_block:
                if stripped.startswith("```"):
                    in_block = True
                    language = stripped[3:].strip()
                    current_lines = []
                else:
                    if stripped:
                        has_non_code_text = True
                continue

            if stripped.startswith("```"):
                code = "\n".join(current_lines).strip("\n")
                if code:
                    blocks.append((language, code))
                in_block = False
                language = ""
                current_lines = []
                continue

            current_lines.append(line)

        # Keep content if the final fence is missing
        if in_block and current_lines:
            code = "\n".join(current_lines).strip("\n")
            if code:
                blocks.append((language, code))

        return blocks, has_non_code_text

    def _should_strip_fenced_code(self, filename: str, has_non_code_text: bool) -> bool:
        """
        Strip fences only when paste content is code-only and not explicitly markdown.
        """
        if has_non_code_text:
            return False
        ext = os.path.splitext(filename)[1].lower()
        if ext in (".md", ".markdown"):
            return False
        return True

    def _resolve_filename(self, filename: str, inferred_ext: Optional[str]) -> str:
        """Ensure filename has an extension and prefer inferred code extension for generic names."""
        if not filename:
            filename = "paste"

        base, ext = os.path.splitext(filename)
        ext = ext.lower()

        # No extension: prefer inferred code extension, otherwise txt
        if not ext:
            return f"{filename}.{inferred_ext or 'txt'}"

        # Generic extensions: promote to inferred language extension if available
        if inferred_ext and ext in (".txt", ".text"):
            return f"{base}.{inferred_ext}"

        return filename

    def _prepare_paste_content(self, content: str, filename: str) -> Tuple[str, str]:
        """
        Prepare content and filename for upload.

        If content is purely fenced code blocks, strip fences and infer extension.
        Otherwise preserve original content.
        """
        blocks, has_non_code_text = self._extract_fenced_blocks_with_context(content)
        if not blocks:
            return content, self._resolve_filename(filename, None)

        if not self._should_strip_fenced_code(filename, has_non_code_text):
            return content, self._resolve_filename(filename, None)

        cleaned_content = "\n\n".join(block for _, block in blocks).strip("\n")
        primary_language, _ = max(blocks, key=lambda item: len(item[1]))
        inferred_ext = self._language_to_extension(primary_language)
        resolved_filename = self._resolve_filename(filename, inferred_ext)
        return f"{cleaned_content}\n", resolved_filename
    
    def execute(
        self,
        content: str,
        filename: str = "paste.txt",
        retention: str = "1week",
        **kwargs
    ) -> str:
        """
        Create a paste on botbin.net.

        Args:
            content: The text/code to paste
            filename: Filename with extension (default: paste.txt)
            retention: Retention time - 1day, 1week, or 1month (default: 1week)

        Returns:
            URL to the paste or error message
        """
        if not self.api_key:
            return "Error: BOTBIN_API_KEY not configured"

        # Validate retention
        if retention not in self.VALID_EXPIRIES:
            retention = "1week"

        # Validate content
        if not content or not content.strip():
            return "Error: No content provided to paste"

        content, filename = self._prepare_paste_content(content, filename)

        try:
            # Write content to temp file
            file_suffix = os.path.splitext(filename)[1] or ".txt"
            with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", delete=False, suffix=file_suffix) as tmp:
                tmp.write(content)
                tmp_path = tmp.name

            try:
                # Upload to botbin
                retention_hours = self.EXPIRY_MAP.get(retention, "168h")
                with open(tmp_path, "rb") as f:
                    response = requests.post(
                        self.api_url,
                        headers={"Authorization": f"Bearer {self.api_key}"},
                        files={"file": (filename, f)},
                        data={"retention": retention_hours},
                        timeout=30,
                    )

                if response.status_code not in (200, 201):
                    return f"Error: Paste failed - {response.status_code} {response.text}"

                # Response is JSON with url field
                result = response.json()
                url = result.get("url")
                if not url:
                    return f"Error: No URL in response: {result}"

                return url
            finally:
                os.unlink(tmp_path)

        except requests.exceptions.Timeout:
            return "Error: Paste request timed out"
        except requests.exceptions.RequestException as e:
            return f"Error: Paste request failed - {str(e)}"
        except Exception as e:
            return f"Error: {str(e)}"
