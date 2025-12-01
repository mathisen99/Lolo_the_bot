"""
Lolo IRC Bot - Python API Server

This is the main entry point for the Python API that handles extensible bot commands
and mention responses. The Go bot communicates with this API via HTTP.
"""

import time
from contextlib import asynccontextmanager
from fastapi import FastAPI
from rich.console import Console

from api.router import router
from api.loader import CommandLoader

# Initialize rich console for colored output
console = Console()

# Track startup time for uptime calculation
startup_time = time.time()

# Global command loader instance
command_loader = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Lifespan context manager for startup and shutdown events.
    """
    global command_loader
    
    # Startup
    console.print("[bold green]Starting Lolo Python API...[/bold green]")
    
    # Initialize command loader
    command_loader = CommandLoader()
    command_loader.load_commands()
    
    console.print(f"[green]✓[/green] Loaded {len(command_loader.commands)} commands")
    console.print("[bold green]API server ready![/bold green]")
    
    yield
    
    # Shutdown
    console.print("[bold yellow]Shutting down Lolo Python API...[/bold yellow]")


# Create FastAPI application
app = FastAPI(
    title="Lolo IRC Bot API",
    description="Extensible command API for the Lolo IRC bot",
    version="1.0.0",
    lifespan=lifespan
)

# Include router
app.include_router(router)


@app.get("/")
async def root():
    """
    Root endpoint - basic API information.
    """
    return {
        "name": "Lolo IRC Bot API",
        "version": "1.0.0",
        "status": "running"
    }


def get_uptime() -> float:
    """
    Get API uptime in seconds.
    """
    return time.time() - startup_time


def get_command_loader() -> CommandLoader:
    """
    Get the global command loader instance.
    """
    return command_loader


if __name__ == "__main__":
    import uvicorn
    
    console.print("[bold cyan]Starting Lolo Python API in development mode...[/bold cyan]")
    uvicorn.run(
        "api.main:app",
        host="0.0.0.0",
        port=8000,
        reload=True,
        log_level="info"
    )
