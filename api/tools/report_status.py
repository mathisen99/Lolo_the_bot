"""
Report Status tool implementation.

Allows the AI to report its current status/activity to the user during long-running tasks.
"""

from typing import Any, Dict, Optional
from .base import Tool
from .null_response import NULL_RESPONSE_MARKER as BASE_NULL_MARKER

# Special marker to indicate a status update that shouldn't break the reasoning chain
STATUS_UPDATE_MARKER = "<<STATUS_UPDATE>>"

class ReportStatusTool(Tool):
    """
    Tool for reporting status updates to the user.
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
            "description": "Report your current status or what you are doing to the user. Use this when performing multi-step tasks, research, or when an operation might take time. This keeps the user informed without stopping your work. Example: 'Reading the abstract of the paper...', 'Searching for counter-arguments...', 'Analyzing the code...'",
            "parameters": {
                "type": "object",
                "properties": {
                    "status_message": {
                        "type": "string",
                        "description": "The concise status message to show the user (e.g., 'Searching for X', 'Reading file Y')"
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
