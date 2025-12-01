"""
Paste tool implementation.

Provides text/code pasting to bpa.st for content that doesn't work well on IRC.
"""

import requests
from typing import Any, Dict, Optional, List
from .base import Tool


class PasteTool(Tool):
    """Paste tool using bpa.st API."""
    
    # Common lexers for quick reference
    COMMON_LEXERS = {
        "python", "javascript", "typescript", "go", "rust", "c", "cpp", 
        "java", "bash", "shell", "json", "yaml", "toml", "xml", "html", 
        "css", "sql", "markdown", "text"
    }
    
    VALID_EXPIRIES = {"1day", "1week", "1month"}
    
    def __init__(self):
        """Initialize paste tool."""
        self.api_url = "https://bpa.st/api/v1/paste"
    
    @property
    def name(self) -> str:
        return "create_paste"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "create_paste",
            "description": "Create a paste on bpa.st for content that doesn't work well on IRC (code, long text, formatted content). Use this when your response would exceed 3 IRC messages or contains code/formatted text that needs proper display. Returns a short URL to the paste.",
            "parameters": {
                "type": "object",
                "properties": {
                    "content": {
                        "type": "string",
                        "description": "The text or code content to paste"
                    },
                    "lexer": {
                        "type": "string",
                        "description": "Syntax highlighting language. Common: python, javascript, go, rust, bash, json, yaml, text. Default: text"
                    },
                    "filename": {
                        "type": "string",
                        "description": "Optional filename for the paste (e.g., 'example.py')"
                    },
                    "expiry": {
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
        lexer: str = "text",
        filename: Optional[str] = None,
        expiry: str = "1week",
        **kwargs
    ) -> str:
        """
        Create a paste on bpa.st.
        
        Args:
            content: The text/code to paste
            lexer: Syntax highlighting language (default: text)
            filename: Optional filename for the paste
            expiry: Expiry time - 1day, 1week, or 1month (default: 1week)
            
        Returns:
            URL to the paste or error message
        """
        # Validate expiry
        if expiry not in self.VALID_EXPIRIES:
            expiry = "1week"
        
        # Validate content
        if not content or not content.strip():
            return "Error: No content provided to paste"
        
        try:
            # Build file object
            file_obj: Dict[str, str] = {
                "lexer": lexer,
                "content": content
            }
            
            if filename:
                file_obj["name"] = filename
            
            # Build request payload
            payload = {
                "expiry": expiry,
                "files": [file_obj]
            }
            
            # Make API request
            response = requests.post(
                self.api_url,
                headers={"Content-Type": "application/json"},
                json=payload,
                timeout=15
            )
            
            if response.status_code != 200:
                return f"Error: Paste failed - {response.status_code} {response.text}"
            
            result = response.json()
            
            if "link" not in result:
                return f"Error: Unexpected response from paste server: {result}"
            
            return result["link"]
            
        except requests.exceptions.Timeout:
            return "Error: Paste request timed out"
        except requests.exceptions.RequestException as e:
            return f"Error: Paste request failed - {str(e)}"
        except Exception as e:
            return f"Error: {str(e)}"
