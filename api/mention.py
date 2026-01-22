"""
Bot mention handler for the Lolo Python API.

Processes messages where the bot is mentioned in a channel and generates
AI-powered responses using GPT-5.1 with web search and Python execution tools.
"""

from api.router import MentionRequest, MentionResponse
from api.utils.output import log_info, log_success, log_error, log_warning
from api.ai import AIClient, AIConfig
from api.tools import NULL_RESPONSE_MARKER

# Global AI client instance (initialized on first use)
_ai_client = None


def get_ai_client() -> AIClient:
    """
    Get or create the global AI client instance.
    
    Returns:
        AIClient instance
    """
    global _ai_client
    if _ai_client is None:
        try:
            config = AIConfig()
            _ai_client = AIClient(config)
            log_success("AI client initialized successfully")
        except Exception as e:
            log_error(f"Failed to initialize AI client: {e}")
            raise
    return _ai_client


def handle_mention(request: MentionRequest) -> MentionResponse:
    """
    Handle a bot mention with AI-powered response.
    
    Uses GPT-5.2 to generate contextual responses with support for:
    - Web search for current information
    - Python execution for calculations and code examples
    - Concise responses optimized for IRC (max 3 messages)
    - Conversation history for context-aware responses
    - Deep research mode for thorough, well-researched answers
    
    Args:
        request: MentionRequest containing mention details and conversation history
        
    Returns:
        MentionResponse with the bot's AI-generated reply
    """
    log_info(f"[{request.request_id}] Processing mention from {request.nick} in {request.channel}" + 
             (" [DEEP MODE]" if request.deep_mode else ""))
    
    # Log conversation history if provided
    if request.history:
        log_info(f"[{request.request_id}] Received {len(request.history)} messages of conversation history")
    
    try:
        # Get AI client
        ai_client = get_ai_client()
        
        # Generate AI response with conversation history
        response_message = ai_client.generate_response_with_context(
            user_message=request.message,
            nick=request.nick,
            channel=request.channel,
            conversation_history=request.history,
            permission_level=request.permission_level,
            request_id=request.request_id,
            deep_mode=request.deep_mode
        )
        
        # Check for null response (user requested silence)
        if response_message == NULL_RESPONSE_MARKER:
            log_success(f"[{request.request_id}] Null response - staying silent for {request.nick}")
            return MentionResponse(
                request_id=request.request_id,
                status="null",
                message=""
            )
        
        log_success(f"[{request.request_id}] Generated AI response for {request.nick}")
        
        return MentionResponse(
            request_id=request.request_id,
            status="success",
            message=response_message
        )
        
    except Exception as e:
        log_error(f"[{request.request_id}] Error processing mention: {e}")
        
        # Fallback to simple response on error
        fallback_message = generate_fallback_response(request.nick, request.message)
        
        return MentionResponse(
            request_id=request.request_id,
            status="error",
            message=fallback_message
        )


def generate_fallback_response(nick: str, message: str) -> str:
    """
    Generate a simple fallback response when AI is unavailable.
    
    Args:
        nick: Nickname of the user who mentioned the bot
        message: The message content
        
    Returns:
        Simple fallback response
    """
    log_warning("Using fallback response (AI unavailable)")
    
    message_lower = message.lower()
    
    if any(word in message_lower for word in ["hello", "hi", "hey"]):
        return f"Hello {nick}! I'm having trouble with my AI right now, but I'm here!"
    elif "?" in message:
        return f"{nick}: I'd love to help, but my AI is temporarily unavailable. Please try again later!"
    else:
        return f"Hi {nick}! I'm experiencing technical difficulties. Please try again in a moment."
