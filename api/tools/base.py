"""
Base tool interface.

Defines the common interface for all AI tools.
"""

from abc import ABC, abstractmethod
from typing import Any, Dict


class Tool(ABC):
    """Base class for AI tools."""
    
    @abstractmethod
    def get_definition(self) -> Dict[str, Any]:
        """
        Get the tool definition for OpenAI API.
        
        Returns:
            Tool definition dict compatible with OpenAI Responses API
        """
        pass
    
    @property
    @abstractmethod
    def name(self) -> str:
        """Get the tool name."""
        pass
