"""
Echo command for the Lolo IRC bot.

Echoes back the provided arguments. Demonstrates required_level field usage.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata, ArgumentSchema
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the echo command.
    """
    return CommandMetadata(
        name="echo",
        help_text="Echo command - repeats the provided text (requires normal user level)",
        required_permission="normal",
        arguments=[
            ArgumentSchema(
                name="text",
                type="string",
                required=True,
                description="Text to echo back"
            )
        ],
        timeout=10,
        cooldown=2
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the echo command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with echoed message
    """
    log_info(f"[{request.request_id}] Executing echo command for {request.nick}")
    
    # Arguments are already validated by the router
    echo_text = " ".join(request.args)
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message=f"Echo: {echo_text}",
        required_level="normal"  # Demonstrate required_level field
    )
