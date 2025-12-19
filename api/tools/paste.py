"""
Paste tool implementation.

Provides text/code pasting to botbin.net for content that doesn't work well on IRC.
"""

import os
import tempfile
import requests
from typing import Any, Dict, Optional, List
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
                        "description": "Filename for the paste with extension (e.g., 'example.py', 'code.js', 'notes.txt'). Extension determines content type display."
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

        # Ensure filename has extension
        if "." not in filename:
            filename = f"{filename}.txt"

        try:
            # Write content to temp file
            with tempfile.NamedTemporaryFile(mode="w", delete=False) as tmp:
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
