"""
Video generation rate limiter.

Limits video generation to 4 videos per 24 hours for normal users.
Owner bypasses the limit entirely and their generations don't count.
"""

import time
from typing import Tuple
from threading import Lock

MAX_VIDEOS_PER_DAY = 4
WINDOW_SECONDS = 86400  # 24 hours

_video_timestamps: list = []
_lock = Lock()


def check_video_rate_limit(permission_level: str) -> Tuple[bool, str]:
    """
    Check if a video generation is allowed.

    Owner always allowed and doesn't affect the shared pool.
    Normal/admin users share a pool of 4/day.
    """
    if permission_level == "owner":
        return True, ""

    with _lock:
        now = time.time()
        cutoff = now - WINDOW_SECONDS

        global _video_timestamps
        _video_timestamps = [ts for ts in _video_timestamps if ts > cutoff]

        if len(_video_timestamps) >= MAX_VIDEOS_PER_DAY:
            remaining = int((_video_timestamps[0] + WINDOW_SECONDS) - now)
            hours = remaining // 3600
            minutes = (remaining % 3600) // 60
            return False, f"All {MAX_VIDEOS_PER_DAY} of {MAX_VIDEOS_PER_DAY} videos used today. Try again in ~{hours}h {minutes}m."

        return True, ""


def record_video_generation() -> None:
    """Record a video generation (call after successful generation, NOT for owner)."""
    with _lock:
        _video_timestamps.append(time.time())


def get_remaining_video_quota() -> int:
    """Get remaining video quota for the current 24h window."""
    with _lock:
        now = time.time()
        cutoff = now - WINDOW_SECONDS

        global _video_timestamps
        _video_timestamps = [ts for ts in _video_timestamps if ts > cutoff]

        return max(0, MAX_VIDEOS_PER_DAY - len(_video_timestamps))
