"""
Null response tool implementation.

Allows the AI to intentionally not respond when users request silence.
Returns a special marker that the client handles to suppress output.
"""

from typing import Any, Dict
from .base import Tool


# Special marker that indicates "do not send any IRC message"
NULL_RESPONSE_MARKER = "<<NULL_RESPONSE>>"


class NullResponseTool(Tool):
    """Tool for intentionally not responding to a message."""
    
    @property
    def name(self) -> str:
        return "null_response"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "null_response",
            "description": """Use this tool when the user explicitly asks you NOT to respond, stay silent, or ignore their message.

Examples of when to use this:
- "Lolo don't respond to this"
- "Lolo ignore this message"
- "Lolo stay quiet"
- "Lolo shh"
- "Lolo no reply please"
- "don't say anything Lolo"

This will cause NO message to be sent to IRC - complete silence.

Do NOT use this for:
- Normal questions or requests
- When user is just being rude (respond politely instead)
- When you're unsure what to do (ask for clarification instead)

Only use when the user EXPLICITLY requests no response.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "reason": {
                        "type": "string",
                        "description": "Brief reason why staying silent (for logging purposes only)."
                    }
                },
                "required": ["reason"],
                "additionalProperties": False
            }
        }
    
    def execute(self, reason: str = "User requested silence", **kwargs) -> str:
        """
        Return the null response marker.
        
        Args:
            reason: Why the AI is staying silent (logged but not sent)
            
        Returns:
            Special marker that client interprets as "send nothing"
        """
        from api.utils.output import log_info
        log_info(f"[NULL_RESPONSE] Staying silent: {reason}")
        return NULL_RESPONSE_MARKER
