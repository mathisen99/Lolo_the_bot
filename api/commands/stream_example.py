"""
Example streaming command for the Lolo Python API.

This command demonstrates how to implement a streaming response
that returns multiple chunks progressively.
"""

from typing import List, Generator, Dict, Any
from api.router import CommandRequest, CommandResponse, ArgumentSchema, CommandMetadata


def get_metadata() -> CommandMetadata:
    """Return metadata for the stream_example command."""
    return CommandMetadata(
        name="stream_example",
        help_text="Example streaming command that returns multiple chunks",
        required_permission="any",
        arguments=[
            ArgumentSchema(
                name="count",
                type="int",
                required=False,
                description="Number of chunks to return (default: 3)",
                default=3
            )
        ],
        timeout=30,
        cooldown=1,
        streaming=True  # Mark this command as supporting streaming
    )


def handle(request: CommandRequest) -> Generator[Dict[str, Any], None, None]:
    """
    Handle a streaming command request.
    
    Returns a generator that yields response chunks.
    Each chunk should be a dictionary with at least:
    - status: "success" or "error"
    - message: the chunk content
    - streaming: True if more chunks follow, False for the final chunk
    
    Args:
        request: CommandRequest with command details
    
    Yields:
        Dict with response chunk data
    """
    try:
        # Parse the count argument
        count = 3
        if request.args and len(request.args) > 0:
            try:
                count = int(request.args[0])
                if count < 1 or count > 100:
                    count = 3
            except (ValueError, IndexError):
                count = 3
        
        # Generate and yield chunks
        for i in range(1, count + 1):
            is_final = (i == count)
            
            yield {
                "request_id": request.request_id,
                "status": "success",
                "message": f"Chunk {i}/{count}: This is streaming chunk number {i}",
                "streaming": not is_final  # False for the final chunk
            }
    
    except Exception as e:
        # Return error chunk
        yield {
            "request_id": request.request_id,
            "status": "error",
            "message": f"Streaming error: {str(e)}",
            "streaming": False
        }
