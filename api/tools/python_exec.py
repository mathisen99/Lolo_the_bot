"""
Python execution tool implementation.

Provides safe Python code execution using OpenAI's built-in code_interpreter tool.
"""

from typing import Any, Dict
from .base import Tool


class PythonExecTool(Tool):
    """Python code execution tool using OpenAI's built-in code_interpreter."""
    
    @property
    def name(self) -> str:
        return "code_interpreter"
    
    def get_definition(self) -> Dict[str, Any]:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict for code_interpreter
        """
        return {
            "type": "code_interpreter",
            "container": {"type": "auto"}
        }
