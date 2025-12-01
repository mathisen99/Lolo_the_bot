"""
Announce command for the Lolo IRC bot.

Allows admins to make formatted announcements. Demonstrates admin-level
permissions and argument validation.
"""

from api.router import CommandRequest, CommandResponse, CommandMetadata, ArgumentSchema
from api.utils.output import log_info


def get_metadata() -> CommandMetadata:
    """
    Return metadata for the announce command.
    
    This command demonstrates:
    - Admin-level permission requirement
    - Multiple required arguments
    - Longer timeout for processing
    """
    return CommandMetadata(
        name="announce",
        help_text="Make a formatted announcement (admin only). Usage: !announce <title> <message>",
        required_permission="admin",
        arguments=[
            ArgumentSchema(
                name="title",
                type="string",
                required=True,
                description="Announcement title"
            ),
            ArgumentSchema(
                name="message",
                type="string",
                required=True,
                description="Announcement message"
            )
        ],
        timeout=20,
        cooldown=30  # Long cooldown to prevent announcement spam
    )


def handle(request: CommandRequest) -> CommandResponse:
    """
    Handle the announce command.
    
    Args:
        request: CommandRequest with command details
        
    Returns:
        CommandResponse with formatted announcement
    """
    log_info(f"[{request.request_id}] Executing announce command for {request.nick}")
    
    # Validate arguments
    if len(request.args) < 2:
        return CommandResponse(
            request_id=request.request_id,
            status="error",
            message="Usage: !announce <title> <message>"
        )
    
    # Parse arguments
    title = request.args[0]
    message = " ".join(request.args[1:])
    
    # Validate title length
    if len(title) > 50:
        return CommandResponse(
            request_id=request.request_id,
            status="error",
            message="Title must be 50 characters or less"
        )
    
    # Validate message length
    if len(message) > 300:
        return CommandResponse(
            request_id=request.request_id,
            status="error",
            message="Message must be 300 characters or less"
        )
    
    # Format announcement
    announcement = f"📢 [{title.upper()}] {message}"
    
    return CommandResponse(
        request_id=request.request_id,
        status="success",
        message=announcement,
        required_level="admin"
    )
