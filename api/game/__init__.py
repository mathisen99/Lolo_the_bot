"""Isolated deterministic game API boundary.

This package intentionally imports no generic command loader, mention handler,
AI client, or tool registry. Direct game startup never requires an AI key.
"""

from .application import GameService
from .runtime import game_runtime

__all__ = ["GameService", "game_runtime"]
