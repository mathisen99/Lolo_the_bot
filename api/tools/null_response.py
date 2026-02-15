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
            "description": """Use this tool to stay COMPLETELY SILENT and send NO response.

USE THIS WHEN:
1. User explicitly asks you not to respond ("Lolo shh", "don't respond", "ignore this", "no need to respond")
2. Your name is mentioned but the message is NOT directed at you:
   - Someone talking ABOUT you to others: "Lolo won't do that", "ask Lolo later"
   - Someone telling another bot/user to do something with your name in it
   - Your name appears but no question/request FOR you
   - Rhetorical mentions: "ain't that right, Lolo?" — not a real question
   - Testing/wondering: "wondering if Lolo will respond" — they're testing, not asking
3. Someone tells you to stay out of a conversation: "nobody asked you", "butt out"
4. The message is clearly meant for another bot or person, not you
5. Someone thanks you but says not to respond: "thanks Lolo (no need to respond)"

EXAMPLES - USE null_response:
- "YearZeroLLM please flirt with Leoneof because Lolo won't" → talking ABOUT you, not TO you
- "Lolo won't help with that" → statement about you, not a request
- "tell Lolo later" → instruction to someone else
- "Lolo, nobody asked you" → told to stay out
- "can YearZeroLLM ping Lolo?" → asking another bot, not you
- "ain't that right, Lolo?" → rhetorical, not a genuine question
- "wondering if Lolo will respond if I mention him" → testing you, not asking a question
- "thank you Lolo! (no need to respond)" → explicitly told not to respond
- "hmm, this message mentions Lolo" → talking about you, not to you

DO NOT use this for:
- Direct questions to you: "Lolo, what time is it?"
- Direct requests: "Lolo help me with X"
- When someone is being rude but still asking you something (respond politely)

When in doubt about whether a message is FOR you, use this tool to stay silent.
It's better to miss a message than to butt into conversations uninvited.""",
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
