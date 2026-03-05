"""
AI module for Lolo bot.

Handles AI-powered responses using the configured OpenAI model with tool support.
"""

from .client import AIClient
from .config import AIConfig

__all__ = ["AIClient", "AIConfig"]
