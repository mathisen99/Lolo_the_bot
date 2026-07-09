"""
Per-user question rate limiter for specific channels.

Limits how many questions a user can ask in a given time window per channel.
Configured via [channel_limits] in ai_settings.toml.
Tracks by hostmask (not nick) to prevent bypass via nick changes.
"""

import time
from typing import Dict, Tuple, Optional
from threading import Lock
from pathlib import Path
import tomli

from api.utils.output import log_info, log_warning


# Storage: {channel: {hostmask: [timestamp, timestamp, ...]}}
_question_counts: Dict[str, Dict[str, list]] = {}
_lock = Lock()

# Loaded config: {channel_name: {max_questions, window_minutes, message}}
_channel_limits: Dict[str, dict] = {}
_config_loaded = False


def _load_config() -> None:
    """Load channel_limits from ai_settings.toml."""
    global _channel_limits, _config_loaded
    
    config_path = Path(__file__).parent.parent / "config" / "ai_settings.toml"
    try:
        with open(config_path, "rb") as f:
            config = tomli.load(f)
        
        limits_section = config.get("channel_limits", {})
        
        for channel_name, settings in limits_section.items():
            if isinstance(settings, dict):
                _channel_limits[channel_name] = {
                    "max_questions": settings.get("max_questions", 2),
                    "window_minutes": settings.get("window_minutes", 60),
                    "message": settings.get("message", 
                        "You've reached the question limit for this channel. Try again later!"),
                }
        
        _config_loaded = True
        if _channel_limits:
            log_info(f"Channel rate limits loaded for: {', '.join(_channel_limits.keys())}")
    except Exception as e:
        log_warning(f"Failed to load channel rate limits: {e}")
        _config_loaded = True


def _get_channel_key(channel: str) -> Optional[str]:
    """
    Match a channel name to a configured limit key.
    Channel comes in as '#windows', config key is 'windows'.
    """
    # Strip leading # for matching
    clean = channel.lstrip("#").lower()
    
    for key in _channel_limits:
        if key.lower() == clean:
            return key
    
    return None


def check_channel_question_limit(
    nick: str, 
    channel: str, 
    permission_level: str,
    hostmask: Optional[str] = None
) -> Tuple[bool, str]:
    """
    Check if a user is allowed to ask another question in this channel.
    If allowed, immediately records the question to prevent race conditions
    from concurrent requests.
    
    Tracks by hostmask to prevent bypass via nick changes.
    Falls back to nick if hostmask is not available.
    
    Args:
        nick: User's IRC nickname
        channel: Channel name (e.g. '#windows')
        permission_level: User's permission level (owner, admin, normal, ignored)
        hostmask: User's hostmask for reliable identification
        
    Returns:
        Tuple of (allowed: bool, limit_message: str)
        If allowed, limit_message is empty.
    """
    global _config_loaded
    
    if not _config_loaded:
        _load_config()
    
    # Channel question limits apply to ALL users (no exemptions)
    # This keeps support channels clean for everyone
    
    # Check if this channel has a configured limit
    channel_key = _get_channel_key(channel)
    if channel_key is None:
        return True, ""
    
    limit_config = _channel_limits[channel_key]
    max_questions = limit_config["max_questions"]
    window_seconds = limit_config["window_minutes"] * 60
    limit_message = limit_config["message"]
    
    # Use hostmask for tracking (prevents nick-change bypass)
    # Fall back to nick if hostmask not available
    identity = hostmask.lower() if hostmask else nick.lower()
    
    with _lock:
        now = time.time()
        cutoff = now - window_seconds
        
        # Initialize storage if needed
        if channel_key not in _question_counts:
            _question_counts[channel_key] = {}
        
        if identity not in _question_counts[channel_key]:
            _question_counts[channel_key][identity] = []
        
        # Clean old timestamps
        _question_counts[channel_key][identity] = [
            ts for ts in _question_counts[channel_key][identity] 
            if ts > cutoff
        ]
        
        # Check if limit reached
        current_count = len(_question_counts[channel_key][identity])
        if current_count >= max_questions:
            log_warning(
                f"Channel rate limit hit: {nick} ({identity}) in #{channel_key} "
                f"({current_count}/{max_questions} in {limit_config['window_minutes']}min)"
            )
            return False, limit_message
        
        # Record immediately to prevent race conditions from concurrent requests
        _question_counts[channel_key][identity].append(now)
        log_info(
            f"Channel question recorded: {nick} ({identity}) in #{channel_key} "
            f"({current_count + 1}/{max_questions})"
        )
        
        return True, ""


def reload_config() -> None:
    """Force reload of channel limits config."""
    global _config_loaded
    _config_loaded = False
    _channel_limits.clear()
    _load_config()
