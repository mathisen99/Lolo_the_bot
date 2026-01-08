"""
Global image generation rate limiter.

Limits image generation/editing to 3 images per hour across ALL users.
Owner and admin users bypass this limit.
"""

import time
from typing import Tuple
from threading import Lock

# Global rate limit: 3 images per hour
MAX_IMAGES_PER_HOUR = 3
WINDOW_SECONDS = 3600  # 1 hour

# Thread-safe storage for image generation timestamps
_image_timestamps: list = []
_lock = Lock()

# Image tool names that count toward the limit
IMAGE_TOOLS = frozenset([
    "flux_create_image",
    "flux_edit_image",
    "gpt_image",
    "gemini_image",
])


def is_image_tool(tool_name: str) -> bool:
    """Check if a tool is an image generation/editing tool."""
    return tool_name in IMAGE_TOOLS


def check_image_rate_limit(permission_level: str) -> Tuple[bool, str]:
    """
    Check if an image generation is allowed.
    
    Args:
        permission_level: User's permission level (owner, admin, normal, ignored)
        
    Returns:
        Tuple of (allowed: bool, error_message: str)
        If allowed, error_message is empty.
    """
    # Owner and admin bypass the limit
    if permission_level in ("owner", "admin"):
        return True, ""
    
    with _lock:
        now = time.time()
        cutoff = now - WINDOW_SECONDS
        
        # Clean old timestamps
        global _image_timestamps
        _image_timestamps = [ts for ts in _image_timestamps if ts > cutoff]
        
        # Check if limit reached
        if len(_image_timestamps) >= MAX_IMAGES_PER_HOUR:
            return False, "Rate limit reached! Mathisen put a limit on image generation. Try again later."
        
        return True, ""


def record_image_generation() -> None:
    """Record that an image was generated (call after successful generation)."""
    with _lock:
        _image_timestamps.append(time.time())


def get_remaining_quota() -> int:
    """Get remaining image quota for the current hour (for debugging)."""
    with _lock:
        now = time.time()
        cutoff = now - WINDOW_SECONDS
        
        # Clean old timestamps
        global _image_timestamps
        _image_timestamps = [ts for ts in _image_timestamps if ts > cutoff]
        
        return max(0, MAX_IMAGES_PER_HOUR - len(_image_timestamps))
