"""
Stats command for the Lolo IRC bot.

Demonstrates integer argument validation.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata, ArgumentSchema
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the stats command.
    """
    return CommandMetadata(
        name="stats",
        help_text="Get statistics - Usage: !stats <days>",
        required_permission="any",
        arguments=[
            ArgumentSchema(
                name="days",
                type="int",
                required=True,
                description="Number of days to show statistics for"
            )
        ],
        timeout=15,
        cooldown=5
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the stats command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with statistics
    """
    log_info(f"[{request.request_id}] Executing stats command for {request.nick}")
    
    # Arguments are already validated by the router
    days = int(request.args[0])
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message=f"Statistics for the last {days} day(s): [placeholder data]"
    )
