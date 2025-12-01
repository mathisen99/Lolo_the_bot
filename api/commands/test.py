"""
Test command for the Lolo IRC bot.

This is an example command that demonstrates the command module structure.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the test command.
    """
    return CommandMetadata(
        name="test",
        help_text="Test command - responds with 'test succeeded'",
        required_permission="any",
        timeout=10,
        cooldown=3
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the test command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with success message
    """
    log_info(f"[{request.request_id}] Executing test command for {request.nick}")
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message="test succeeded"
    )
