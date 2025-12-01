"""
Ping command for the Lolo IRC bot.

Simple command that responds with 'pong'.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the ping command.
    """
    return CommandMetadata(
        name="ping",
        help_text="Ping command - responds with 'pong'",
        required_permission="any",
        timeout=5,
        cooldown=1
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the ping command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with pong message
    """
    log_info(f"[{request.request_id}] Executing ping command for {request.nick}")
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message="pong"
    )
