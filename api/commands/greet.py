"""
Greet command for the Lolo IRC bot.

Demonstrates argument validation with required and optional arguments.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata, ArgumentSchema
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the greet command.
    """
    return CommandMetadata(
        name="greet",
        help_text="Greet a user - Usage: !greet <username> [greeting]",
        required_permission="any",
        arguments=[
            ArgumentSchema(
                name="username",
                type="user",
                required=True,
                description="The user to greet"
            ),
            ArgumentSchema(
                name="greeting",
                type="string",
                required=False,
                description="Custom greeting message (optional)",
                default="Hello"
            )
        ],
        timeout=10,
        cooldown=2
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the greet command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with greeting message
    """
    log_info(f"[{request.request_id}] Executing greet command for {request.nick}")
    
    # Arguments are already validated by the router
    username = request.args[0]
    greeting = " ".join(request.args[1:]) if len(request.args) > 1 else "Hello"
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message=f"{greeting}, {username}!"
    )
