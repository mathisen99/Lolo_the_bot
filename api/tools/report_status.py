"""
Report Status tool implementation.

Allows the AI to send one acknowledgement before a long-running task.
"""

from typing import Any, Dict, Optional
from .base import Tool
from .null_response import NULL_RESPONSE_MARKER as BASE_NULL_MARKER

# Special marker to indicate a status update that shouldn't break the reasoning chain
STATUS_UPDATE_MARKER = "<<STATUS_UPDATE>>"

class ReportStatusTool(Tool):
    """
    Tool for acknowledging a long-running task.
    """
    
    @property
    def name(self) -> str:
        return "report_status"
    
    def get_definition(self) -> Dict[str, Any]:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict
        """
        return {
            "type": "function",
            "name": self.name,
            "description": (
                "Send one concise, request-specific acknowledgement before beginning a "
                "genuinely long, multi-step task. It must briefly say why this request "
                "will take time and what concrete checks or actions will happen next. "
                "Never send generic or reusable boilerplate. Call this at most once per "
                "request. Do not use it for simple questions, calculations, or a single "
                "quick web lookup. Further actions are recorded privately and must not "
                "be narrated to the channel."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "status_message": {
                        "type": "string",
                        "description": (
                            "A fresh, short sentence tied to the request that states both "
                            "why the work may take a while and the concrete next actions."
                        )
                    }
                },
                "required": ["status_message"]
            }
        }
    
    def execute(self, status_message: str, **kwargs) -> str:
        """
        Execute the tool.
        
        Args:
            status_message: The status message to report
            
        Returns:
            Status update marker with message
        """
        # We return a formatted string that the client will parse
        # The client will strip this marker, send the status to the user, 
        # and return a "Status reported" message to the LLM to continue the chain
        return f"{STATUS_UPDATE_MARKER}{status_message}"
