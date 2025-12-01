"""
Colored output utilities for the Lolo Python API.

Provides consistent colored logging across the API.
"""

from rich.console import Console
from datetime import datetime

console = Console()


def log_info(message: str) -> None:
    """Log an info message in blue."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    console.print(f"[blue][{timestamp}][/blue] {message}")


def log_success(message: str) -> None:
    """Log a success message in green."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    console.print(f"[green][{timestamp}] ✓[/green] {message}")


def log_error(message: str) -> None:
    """Log an error message in red."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    console.print(f"[red][{timestamp}] ✗[/red] {message}")


def log_warning(message: str) -> None:
    """Log a warning message in yellow."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    console.print(f"[yellow][{timestamp}] ⚠[/yellow] {message}")


def log_debug(message: str) -> None:
    """Log a debug message in dim."""
    timestamp = datetime.now().strftime("%H:%M:%S")
    console.print(f"[dim][{timestamp}] DEBUG:[/dim] {message}")
