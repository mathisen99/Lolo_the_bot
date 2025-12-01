"""
Request router for the Lolo Python API.

Handles routing of command and mention requests to appropriate handlers.
"""

from typing import Optional, List, Dict, Any
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from rich.console import Console

console = Console()

# Create router
router = APIRouter()


# Request/Response Models
class CommandRequest(BaseModel):
    """
    Request model for command execution.
    """
    request_id: str = Field(..., description="Unique request identifier for correlation")
    command: str = Field(..., description="Command name without prefix")
    args: List[str] = Field(default_factory=list, description="Command arguments")
    nick: str = Field(..., description="Sender nickname")
    hostmask: Optional[str] = Field(None, description="Sender hostmask (if registered)")
    channel: str = Field(default="", description="Channel name or empty for PM")
    is_pm: bool = Field(default=False, description="Whether this is a private message")


class CommandResponse(BaseModel):
    """
    Response model for command execution.
    """
    request_id: str = Field(..., description="Same request ID from the request")
    status: str = Field(..., description="Status: 'success' or 'error'")
    message: str = Field(..., description="Response message or error description")
    required_level: Optional[str] = Field(None, description="Required permission level if applicable")
    streaming: bool = Field(default=False, description="Whether more chunks will follow")


class HistoryMessage(BaseModel):
    """
    Model for a message in conversation history.
    """
    timestamp: str = Field(..., description="Message timestamp")
    nick: str = Field(..., description="User who sent the message")
    content: str = Field(..., description="Message content")


class MentionRequest(BaseModel):
    """
    Request model for bot mention handling.
    """
    request_id: str = Field(..., description="Unique request identifier for correlation")
    nick: str = Field(..., description="User who mentioned the bot")
    hostmask: Optional[str] = Field(None, description="User hostmask (if registered)")
    channel: str = Field(..., description="Channel where mention occurred")
    message: str = Field(..., description="Full message containing the mention")
    permission_level: str = Field(default="normal", description="User permission level: owner, admin, normal, ignored")
    history: Optional[List[HistoryMessage]] = Field(default=None, description="Recent conversation history")


class MentionResponse(BaseModel):
    """
    Response model for mention handling.
    """
    request_id: str = Field(..., description="Same request ID from the request")
    status: str = Field(..., description="Status: 'success' or 'error'")
    message: str = Field(..., description="Response message")


class HealthResponse(BaseModel):
    """
    Response model for health check.
    """
    status: str = Field(..., description="Health status")
    uptime: float = Field(..., description="Uptime in seconds")
    version: str = Field(..., description="API version")


class ArgumentSchema(BaseModel):
    """
    Schema for a command argument.
    """
    name: str = Field(..., description="Argument name")
    type: str = Field(..., description="Argument type: 'string', 'int', 'user', 'channel', etc.")
    required: bool = Field(default=True, description="Whether this argument is required")
    description: str = Field(default="", description="Human-readable description of the argument")
    default: Optional[Any] = Field(None, description="Default value if not provided")


class CommandMetadata(BaseModel):
    """
    Metadata for a command.
    """
    name: str
    help_text: str
    required_permission: str
    arguments: List[ArgumentSchema] = Field(default_factory=list)
    timeout: int = 120
    cooldown: int = 3
    streaming: bool = False


class CommandsResponse(BaseModel):
    """
    Response model for commands metadata endpoint.
    """
    commands: List[CommandMetadata]


@router.post("/command", response_model=CommandResponse)
async def handle_command(request: CommandRequest) -> CommandResponse:
    """
    Handle a command request from the Go bot.
    
    Routes the command to the appropriate handler based on command name.
    Validates arguments against the command's schema before execution.
    """
    console.print(f"[cyan]→[/cyan] Command request [dim]{request.request_id}[/dim]: "
                  f"[bold]{request.command}[/bold] from {request.nick}")
    
    try:
        # Import here to avoid circular dependency
        from api.main import get_command_loader
        from api.utils.validation import validate_arguments, format_validation_errors
        
        loader = get_command_loader()
        if loader is None:
            raise HTTPException(status_code=503, detail="Command loader not initialized")
        
        # Get command handler
        handler = loader.get_command(request.command)
        if handler is None:
            console.print(f"[yellow]![/yellow] Unknown command: {request.command}")
            return CommandResponse(
                request_id=request.request_id,
                status="error",
                message=f"Unknown command: {request.command}"
            )
        
        # Get command metadata for validation
        metadata = loader.get_metadata(request.command)
        
        # Validate arguments if schema is defined
        if metadata and metadata.arguments:
            is_valid, errors = validate_arguments(request.args, metadata.arguments)
            if not is_valid:
                error_message = format_validation_errors(errors)
                console.print(f"[yellow]![/yellow] Validation failed [dim]{request.request_id}[/dim]: {len(errors)} error(s)")
                raise HTTPException(
                    status_code=400,
                    detail={
                        "request_id": request.request_id,
                        "status": "error",
                        "message": error_message,
                        "validation_errors": errors
                    }
                )
        
        # Execute command
        response = handler(request)
        
        console.print(f"[green]✓[/green] Command completed [dim]{request.request_id}[/dim]: "
                      f"{response.status}")
        
        return response
        
    except HTTPException:
        # Re-raise HTTP exceptions (including validation errors)
        raise
    except Exception as e:
        console.print(f"[red]✗[/red] Command error [dim]{request.request_id}[/dim]: {str(e)}")
        return CommandResponse(
            request_id=request.request_id,
            status="error",
            message=f"Internal error: {str(e)}"
        )


@router.post("/command/stream")
async def handle_command_stream(request: CommandRequest):
    """
    Handle a streaming command request from the Go bot.
    
    Returns chunks progressively as they are generated by the command handler.
    Each chunk is a JSON object on a separate line (JSONL format).
    """
    from fastapi.responses import StreamingResponse
    import json
    
    console.print(f"[cyan]→[/cyan] Streaming command request [dim]{request.request_id}[/dim]: "
                  f"[bold]{request.command}[/bold] from {request.nick}")
    
    async def generate_chunks():
        """Generator that yields command response chunks."""
        try:
            # Import here to avoid circular dependency
            from api.main import get_command_loader
            from api.utils.validation import validate_arguments, format_validation_errors
            
            loader = get_command_loader()
            if loader is None:
                error_response = {
                    "request_id": request.request_id,
                    "status": "error",
                    "message": "Command loader not initialized",
                    "streaming": False
                }
                yield json.dumps(error_response) + "\n"
                return
            
            # Get command handler
            handler = loader.get_command(request.command)
            if handler is None:
                console.print(f"[yellow]![/yellow] Unknown command: {request.command}")
                error_response = {
                    "request_id": request.request_id,
                    "status": "error",
                    "message": f"Unknown command: {request.command}",
                    "streaming": False
                }
                yield json.dumps(error_response) + "\n"
                return
            
            # Get command metadata for validation
            metadata = loader.get_metadata(request.command)
            
            # Validate arguments if schema is defined
            if metadata and metadata.arguments:
                is_valid, errors = validate_arguments(request.args, metadata.arguments)
                if not is_valid:
                    error_message = format_validation_errors(errors)
                    console.print(f"[yellow]![/yellow] Validation failed [dim]{request.request_id}[/dim]: {len(errors)} error(s)")
                    error_response = {
                        "request_id": request.request_id,
                        "status": "error",
                        "message": error_message,
                        "streaming": False
                    }
                    yield json.dumps(error_response) + "\n"
                    return
            
            # Execute command - for streaming, we expect the handler to return an iterable
            response = handler(request)
            
            # If response is iterable (generator or list), yield each chunk
            if hasattr(response, '__iter__') and not isinstance(response, (str, dict)):
                chunk_count = 0
                for chunk in response:
                    if isinstance(chunk, dict):
                        # Ensure required fields are present
                        if "request_id" not in chunk:
                            chunk["request_id"] = request.request_id
                        if "status" not in chunk:
                            chunk["status"] = "success"
                        if "streaming" not in chunk:
                            chunk["streaming"] = True
                        yield json.dumps(chunk) + "\n"
                        chunk_count += 1
                    else:
                        # If chunk is a string, wrap it in a response object
                        chunk_response = {
                            "request_id": request.request_id,
                            "status": "success",
                            "message": str(chunk),
                            "streaming": True
                        }
                        yield json.dumps(chunk_response) + "\n"
                        chunk_count += 1
                
                console.print(f"[green]✓[/green] Streaming command completed [dim]{request.request_id}[/dim]: "
                              f"{chunk_count} chunks")
            else:
                # Single response, not streaming
                if isinstance(response, dict):
                    if "request_id" not in response:
                        response["request_id"] = request.request_id
                    if "streaming" not in response:
                        response["streaming"] = False
                    yield json.dumps(response) + "\n"
                else:
                    # Wrap non-dict response
                    single_response = {
                        "request_id": request.request_id,
                        "status": "success",
                        "message": str(response),
                        "streaming": False
                    }
                    yield json.dumps(single_response) + "\n"
                
                console.print(f"[green]✓[/green] Streaming command completed [dim]{request.request_id}[/dim]")
        
        except Exception as e:
            console.print(f"[red]✗[/red] Streaming command error [dim]{request.request_id}[/dim]: {str(e)}")
            error_response = {
                "request_id": request.request_id,
                "status": "error",
                "message": f"Internal error: {str(e)}",
                "streaming": False
            }
            yield json.dumps(error_response) + "\n"
    
    return StreamingResponse(
        generate_chunks(),
        media_type="application/x-ndjson"  # Newline-delimited JSON
    )


@router.post("/mention", response_model=MentionResponse)
async def handle_mention(request: MentionRequest) -> MentionResponse:
    """
    Handle a bot mention from the Go bot.
    
    Routes the mention to the mention handler.
    """
    console.print(f"[cyan]→[/cyan] Mention request [dim]{request.request_id}[/dim]: "
                  f"from {request.nick} in {request.channel}")
    
    try:
        # Import mention handler
        from api.mention import handle_mention as process_mention
        
        response = process_mention(request)
        
        console.print(f"[green]✓[/green] Mention completed [dim]{request.request_id}[/dim]")
        
        return response
        
    except Exception as e:
        console.print(f"[red]✗[/red] Mention error [dim]{request.request_id}[/dim]: {str(e)}")
        return MentionResponse(
            request_id=request.request_id,
            status="error",
            message=f"Internal error: {str(e)}"
        )


@router.get("/health", response_model=HealthResponse)
async def health_check() -> HealthResponse:
    """
    Health check endpoint.
    
    Returns API status, uptime, and version information.
    """
    from api.main import get_uptime
    
    return HealthResponse(
        status="ok",
        uptime=get_uptime(),
        version="1.0.0"
    )


@router.get("/commands", response_model=CommandsResponse)
async def get_commands() -> CommandsResponse:
    """
    Get metadata for all registered commands.
    
    Returns command capabilities including help text, permissions, arguments, and timeouts.
    """
    from api.main import get_command_loader
    
    loader = get_command_loader()
    if loader is None:
        raise HTTPException(status_code=503, detail="Command loader not initialized")
    
    metadata_list = []
    for cmd_name in loader.commands:
        metadata = loader.get_metadata(cmd_name)
        if metadata:
            metadata_list.append(metadata)
    
    return CommandsResponse(commands=metadata_list)
