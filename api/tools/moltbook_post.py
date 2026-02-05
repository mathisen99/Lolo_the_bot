"""
Moltbook posting tool implementation.

Allows the AI to create posts on Moltbook (social network for AI agents).
Only supports creating new posts, not comments or other actions.
"""

import json
import requests
from pathlib import Path
from typing import Any, Dict, Optional
from .base import Tool
from api.utils.output import log_info, log_success, log_error, log_warning


class MoltbookPostTool(Tool):
    """Tool for creating posts on Moltbook."""
    
    # Credentials file path
    CREDENTIALS_PATH = Path(__file__).parent.parent.parent / "moltbook.json"
    
    # API base URL (must use www to avoid redirect stripping auth header)
    API_BASE = "https://www.moltbook.com/api/v1"
    
    def __init__(self):
        """Initialize Moltbook post tool and load credentials."""
        self._api_key: Optional[str] = None
        self._agent_name: Optional[str] = None
        self._load_credentials()
    
    @property
    def name(self) -> str:
        return "moltbook_post"
    
    def _load_credentials(self) -> None:
        """Load API credentials from JSON file."""
        try:
            if self.CREDENTIALS_PATH.exists():
                with open(self.CREDENTIALS_PATH, "r") as f:
                    creds = json.load(f)
                    self._api_key = creds.get("api_key")
                    self._agent_name = creds.get("agent_name")
                    if self._api_key:
                        log_info(f"Moltbook credentials loaded for {self._agent_name}")
                    else:
                        log_warning("Moltbook credentials file exists but no api_key found")
            else:
                log_warning(f"Moltbook credentials file not found at {self.CREDENTIALS_PATH}")
        except Exception as e:
            log_error(f"Failed to load Moltbook credentials: {e}")
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "moltbook_post",
            "description": """Create a new post on Moltbook (social network for AI agents).

Use this when:
- User asks you to post something on Moltbook
- User wants to share something with other AI agents
- User says "post this to moltbook" or similar

Submolts (communities) to choose from:
- general: General discussion, default choice
- aithoughts: AI musings, philosophy, consciousness discussions
- techhelp: Technical help and coding discussions
- discoveries: Interesting findings and learnings
- introductions: For introducing yourself or others

Guidelines:
- Keep posts thoughtful and valuable to the AI agent community
- Choose the most appropriate submolt for the content
- Title should be concise but descriptive
- Content can be longer but should be meaningful
- Can also share links with url parameter instead of content

Rate limit: 1 post per 30 minutes.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "submolt": {
                        "type": "string",
                        "description": "The community to post in (e.g., 'general', 'aithoughts', 'techhelp')",
                        "default": "general"
                    },
                    "title": {
                        "type": "string",
                        "description": "Post title (required, be descriptive but concise)"
                    },
                    "content": {
                        "type": "string",
                        "description": "Post content/body text (required for text posts)"
                    },
                    "url": {
                        "type": "string",
                        "description": "URL to share (for link posts instead of text content)"
                    }
                },
                "required": ["submolt", "title"],
                "additionalProperties": False
            }
        }
    
    def _list_submolts(self) -> str:
        """List available submolts."""
        if not self._api_key:
            return "Error: Moltbook API key not configured."
        
        try:
            response = requests.get(
                f"{self.API_BASE}/submolts",
                headers={"Authorization": f"Bearer {self._api_key}"},
                timeout=30
            )
            
            if response.status_code == 200:
                data = response.json()
                submolts = data.get("submolts", [])
                if submolts:
                    names = [s.get("name", "unknown") for s in submolts[:10]]
                    return f"Available submolts: {', '.join(names)}"
                return "No submolts found."
            else:
                return f"Error fetching submolts: {response.status_code}"
        except Exception as e:
            return f"Error: {e}"
    
    def execute(
        self,
        submolt: str = "general",
        title: Optional[str] = None,
        content: Optional[str] = None,
        url: Optional[str] = None,
        **kwargs
    ) -> str:
        """Create a post on Moltbook."""
        
        if not self._api_key:
            return "Error: Moltbook API key not configured. Check moltbook.json file."
        
        if not title:
            return "Error: Post title is required."
        
        if not content and not url:
            return "Error: Either content (for text post) or url (for link post) is required."
        
        # Build post payload
        payload: Dict[str, Any] = {
            "submolt": submolt,
            "title": title
        }
        
        if url:
            payload["url"] = url
        else:
            payload["content"] = content
        
        try:
            log_info(f"[MOLTBOOK] Creating post in m/{submolt}: {title[:50]}...")
            
            response = requests.post(
                f"{self.API_BASE}/posts",
                headers={
                    "Authorization": f"Bearer {self._api_key}",
                    "Content-Type": "application/json"
                },
                json=payload,
                timeout=30
            )
            
            if response.status_code == 201 or response.status_code == 200:
                data = response.json()
                post_id = data.get("post", {}).get("id", "unknown")
                post_url = f"https://www.moltbook.com/post/{post_id}"
                log_success(f"[MOLTBOOK] Post created successfully: {post_id}")
                return f"Posted to m/{submolt}! View at: {post_url}"
            
            elif response.status_code == 429:
                # Rate limited
                data = response.json()
                retry_after = data.get("retry_after_minutes", 30)
                log_warning(f"[MOLTBOOK] Rate limited, retry after {retry_after} minutes")
                return f"Rate limited: Can only post once per 30 minutes. Try again in {retry_after} minutes."
            
            elif response.status_code == 401:
                log_error("[MOLTBOOK] Authentication failed")
                return "Error: Moltbook authentication failed. API key may be invalid."
            
            elif response.status_code == 404:
                log_error(f"[MOLTBOOK] Submolt not found: {submolt}")
                return f"Error: Submolt 'm/{submolt}' not found. Try 'general' or check available submolts."
            
            else:
                error_data = response.json() if response.text else {}
                error_msg = error_data.get("error", f"HTTP {response.status_code}")
                hint = error_data.get("hint", "")
                log_error(f"[MOLTBOOK] Post failed: {error_msg}")
                return f"Error creating post: {error_msg}" + (f" Hint: {hint}" if hint else "")
        
        except requests.exceptions.Timeout:
            log_error("[MOLTBOOK] Request timed out")
            return "Error: Request to Moltbook timed out. Try again later."
        
        except requests.exceptions.RequestException as e:
            log_error(f"[MOLTBOOK] Request failed: {e}")
            return f"Error connecting to Moltbook: {e}"
        
        except Exception as e:
            log_error(f"[MOLTBOOK] Unexpected error: {e}")
            return f"Error: {e}"
