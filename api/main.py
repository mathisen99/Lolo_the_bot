"""
Lolo IRC Bot - Python API Server

This is the main entry point for the Python API that handles extensible bot commands
and mention responses. The Go bot communicates with this API via HTTP.
"""

import time
import asyncio
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

# Background task handle
_chroma_task = None

# Migration interval in seconds (15 minutes)
CHROMA_MIGRATE_INTERVAL = 15 * 60


async def run_chroma_migration():
    """
    Run the ChromaDB migration script.
    """
    from scripts.migrate_to_chroma import migrate
    
    try:
        console.print("[cyan]Running ChromaDB migration...[/cyan]")
        # Run in thread pool to avoid blocking
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(None, migrate)
        console.print("[green]✓[/green] ChromaDB migration complete")
    except Exception as e:
        console.print(f"[red]✗[/red] ChromaDB migration failed: {e}")


async def chroma_scheduler():
    """
    Background task that runs ChromaDB migration every 15 minutes.
    """
    # Wait a bit before first run to let the API fully start
    await asyncio.sleep(30)
    
    while True:
        await run_chroma_migration()
        await asyncio.sleep(CHROMA_MIGRATE_INTERVAL)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Lifespan context manager for startup and shutdown events.
    """
    global command_loader, _chroma_task
    
    # Startup
    console.print("[bold green]Starting Lolo Python API...[/bold green]")
    
    # Initialize command loader
    command_loader = CommandLoader()
    command_loader.load_commands()
    
    console.print(f"[green]✓[/green] Loaded {len(command_loader.commands)} commands")
    
    # Start ChromaDB migration scheduler
    _chroma_task = asyncio.create_task(chroma_scheduler())
    console.print(f"[green]✓[/green] ChromaDB scheduler started (every {CHROMA_MIGRATE_INTERVAL // 60} min)")
    
    console.print("[bold green]API server ready![/bold green]")
    
    yield
    
    # Shutdown
    console.print("[bold yellow]Shutting down Lolo Python API...[/bold yellow]")
    
    # Cancel background task
    if _chroma_task:
        _chroma_task.cancel()
        try:
            await _chroma_task
        except asyncio.CancelledError:
            pass
        console.print("[yellow]✓[/yellow] ChromaDB scheduler stopped")


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
