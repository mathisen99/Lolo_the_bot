"""Versioned authored game content and strict loading boundary."""
from .loader import (
    ContentValidationError,
    STANDARD_PROFILE,
    load_standard_campaign,
    load_standard_snapshot,
)

__all__ = [
    "ContentValidationError", "STANDARD_PROFILE", "load_standard_campaign",
    "load_standard_snapshot",
]
