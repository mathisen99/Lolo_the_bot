"""
Deep mode rate limiter.

Limits --deep research queries to 3 per day for normal users.
Owner and admin users bypass this limit.
"""

import time
from typing import Dict, Tuple
from threading import Lock
from datetime import datetime, timedelta

# Global rate limit: 3 deep queries per day per user
MAX_DEEP_PER_DAY = 3

# Thread-safe storage for deep mode usage per user
# Format: {nick: [timestamp1, timestamp2, ...]}
_deep_usage: Dict[str, list] = {}
_lock = Lock()


def check_deep_mode_limit(nick: str, permission_level: str) -> Tuple[bool, str]:
    """
    Check if a deep mode query is allowed for this user.
    
    Args:
        nick: User's nickname
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
        day_ago = now - 86400  # 24 hours in seconds
        
        # Get user's usage, filtering out old entries
        if nick in _deep_usage:
            # Remove entries older than 24 hours
            _deep_usage[nick] = [ts for ts in _deep_usage[nick] if ts > day_ago]
        else:
            _deep_usage[nick] = []
        
        current_count = len(_deep_usage[nick])
        
        if current_count >= MAX_DEEP_PER_DAY:
            # Calculate when the oldest entry expires
            oldest = min(_deep_usage[nick])
            reset_time = datetime.fromtimestamp(oldest + 86400)
            time_until_reset = reset_time - datetime.now()
            hours = int(time_until_reset.total_seconds() // 3600)
            minutes = int((time_until_reset.total_seconds() % 3600) // 60)
            
            return False, f"Deep mode limit reached ({MAX_DEEP_PER_DAY}/day). Resets in {hours}h {minutes}m."
        
        return True, ""


def record_deep_mode_usage(nick: str, permission_level: str) -> None:
    """
    Record a deep mode usage for a user.
    Only records for non-admin/owner users.
    
    Args:
        nick: User's nickname
        permission_level: User's permission level
    """
    # Don't track admin/owner usage
    if permission_level in ("owner", "admin"):
        return
    
    with _lock:
        now = time.time()
        if nick not in _deep_usage:
            _deep_usage[nick] = []
        _deep_usage[nick].append(now)


def get_deep_mode_remaining(nick: str, permission_level: str) -> int:
    """
    Get remaining deep mode queries for a user today.
    
    Args:
        nick: User's nickname
        permission_level: User's permission level
        
    Returns:
        Number of remaining queries (-1 for unlimited/admin)
    """
    if permission_level in ("owner", "admin"):
        return -1  # Unlimited
    
    with _lock:
        now = time.time()
        day_ago = now - 86400
        
        if nick not in _deep_usage:
            return MAX_DEEP_PER_DAY
        
        # Count recent usage
        recent = [ts for ts in _deep_usage[nick] if ts > day_ago]
        return max(0, MAX_DEEP_PER_DAY - len(recent))
