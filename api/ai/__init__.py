"""
AI module for Lolo bot.

Handles AI-powered responses using the configured OpenAI model with tool support.
"""

from typing import Any

__all__ = ["AIClient", "AIConfig"]


def __getattr__(name: str) -> Any:
    """Lazy imports to avoid circular dependencies during tool initialization."""
    if name == "AIClient":
        from .client import AIClient
        return AIClient
    if name == "AIConfig":
        from .config import AIConfig
        return AIConfig
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
