"""
AI module for Lolo bot.

Handles AI-powered responses using GPT-5.1 with tool support.
"""

from .client import AIClient
from .config import AIConfig

__all__ = ["AIClient", "AIConfig"]
