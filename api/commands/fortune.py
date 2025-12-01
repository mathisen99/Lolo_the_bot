"""
Fortune command for the Lolo IRC bot.

Returns a random fortune/quote. Demonstrates a command with no arguments
but with custom timeout and cooldown settings.
"""

import random
from api.router import CommandRequest, CommandResponse, CommandMetadata
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the fortune command.
    
    This command demonstrates:
    - No arguments required
    - Custom cooldown to prevent spam
    - Short timeout for quick responses
    """
    return CommandMetadata(
        name="fortune",
        help_text="Get a random fortune or quote",
        required_permission="any",
        arguments=[],  # No arguments
        timeout=5,
        cooldown=10  # Longer cooldown to prevent fortune spam
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the fortune command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with a random fortune
    """
    log_info(f"[{request.request_id}] Executing fortune command for {request.nick}")
    
    # Collection of fortunes
    fortunes = [
        "A journey of a thousand miles begins with a single step.",
        "The best time to plant a tree was 20 years ago. The second best time is now.",
        "In the middle of difficulty lies opportunity.",
        "The only way to do great work is to love what you do.",
        "Success is not final, failure is not fatal: it is the courage to continue that counts.",
        "Believe you can and you're halfway there.",
        "The future belongs to those who believe in the beauty of their dreams.",
        "It does not matter how slowly you go as long as you do not stop.",
        "Everything you've ever wanted is on the other side of fear.",
        "Hardships often prepare ordinary people for an extraordinary destiny."
    ]
    
    # Select a random fortune
    fortune = random.choice(fortunes)
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message=f"🔮 {fortune}"
    )
