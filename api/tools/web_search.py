"""
Web search tool implementation.

Provides web search capability using OpenAI's built-in web_search tool.
"""

from typing import Any, Dict, List, Optional
from .base import Tool


class WebSearchTool(Tool):
    """Web search tool using OpenAI's built-in web_search."""
    
    def __init__(
        self,
        external_web_access: bool = True,
        allowed_domains: Optional[List[str]] = None
    ):
        """
        Initialize web search tool.
        
        Args:
            external_web_access: Enable live web access (vs cached only)
            allowed_domains: Optional list of allowed domains to search
        """
        self._external_web_access = external_web_access
        self._allowed_domains = allowed_domains or []
    
    @property
    def name(self) -> str:
        return "web_search"
    
    def get_definition(self) -> Dict[str, Any]:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict for web_search
        """
        tool_def = {
            "type": "web_search",
            "external_web_access": self._external_web_access
        }
        
        # Add domain filtering if specified
        if self._allowed_domains:
            tool_def["filters"] = {
                "allowed_domains": self._allowed_domains
            }
        
        return tool_def
    
    def execute(self, *args, **kwargs) -> str:
        """
        Web search is handled by OpenAI API directly.
        This method is not called for built-in tools.
        
        Returns:
            Empty string (not used for built-in tools)
        """
        return ""
