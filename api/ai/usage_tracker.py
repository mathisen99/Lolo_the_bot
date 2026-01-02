"""
Usage tracking for AI API costs.

Logs token usage and calculates costs per request.
"""

import sqlite3
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Optional
from api.utils.output import log_info, log_warning, log_error


# Pricing per 1M tokens
PRICING = {
    "gpt-5.2": {
        "input": 1.75,      # $1.75 per 1M input tokens
        "cached": 0.175,    # $0.175 per 1M cached tokens  
        "output": 14.00,    # $14.00 per 1M output tokens
    },
    "gpt-image-1.5": {
        "input": 5.00,      # $5.00 per 1M input tokens
        "cached": 1.25,     # $1.25 per 1M cached tokens
        "output": 10.00,    # $10.00 per 1M output tokens
    },
    # Add other models as needed
    "default": {
        "input": 2.00,
        "cached": 0.20,
        "output": 10.00,
    }
}

# Per-call costs for native tools
WEB_SEARCH_COST = 0.01       # $10/1k calls = $0.01 each
# Note: code_interpreter removed - now using self-hosted Firecracker (free)


def calculate_cost(model: str, input_tokens: int, cached_tokens: int, output_tokens: int) -> float:
    """
    Calculate cost in USD for token usage.
    
    IMPORTANT: input_tokens is the TOTAL input, cached_tokens is a SUBSET of input_tokens.
    We charge uncached tokens at full price and cached tokens at discounted price.
    """
    pricing = PRICING.get(model, PRICING["default"])
    
    # Cached tokens are a subset of input tokens, not additional
    # Uncached = total input - cached portion
    uncached_tokens = input_tokens - cached_tokens
    
    # Cost calculation (pricing is per 1M tokens)
    uncached_cost = (uncached_tokens / 1_000_000) * pricing["input"]
    cached_cost = (cached_tokens / 1_000_000) * pricing["cached"]
    output_cost = (output_tokens / 1_000_000) * pricing["output"]
    
    return uncached_cost + cached_cost + output_cost


def log_usage(
    request_id: str,
    nick: str,
    channel: Optional[str],
    model: str,
    input_tokens: int,
    cached_tokens: int,
    output_tokens: int,
    tool_calls: int = 0,
    web_search_calls: int = 0,
    code_interpreter_calls: int = 0
) -> None:
    """Log usage to database."""
    db_path = Path("data/bot.db")
    
    if not db_path.exists():
        log_warning(f"[{request_id}] Cannot log usage: database not found")
        return
    
    # Calculate token cost
    token_cost = calculate_cost(model, input_tokens, cached_tokens, output_tokens)
    
    # Add per-call costs for native tools
    web_search_cost = web_search_calls * WEB_SEARCH_COST
    # Note: code_interpreter_calls still logged for historical tracking, but no cost
    
    # Total cost
    cost = token_cost + web_search_cost
    
    try:
        conn = sqlite3.connect(str(db_path))
        cursor = conn.cursor()
        
        cursor.execute("""
            INSERT INTO usage_tracking 
            (timestamp, request_id, nick, channel, model, input_tokens, cached_tokens, output_tokens, cost_usd, tool_calls, web_search_calls, code_interpreter_calls)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
            request_id,
            nick,
            channel,
            model,
            input_tokens,
            cached_tokens,
            output_tokens,
            cost,
            tool_calls,
            web_search_calls,
            code_interpreter_calls
        ))
        
        conn.commit()
        conn.close()
        
        log_info(f"[{request_id}] Usage logged: {input_tokens} in, {cached_tokens} cached, {output_tokens} out = ${cost:.4f}")
        
    except sqlite3.Error as e:
        log_warning(f"[{request_id}] Failed to log usage: {e}")
    except Exception as e:
        log_warning(f"[{request_id}] Unexpected error logging usage: {e}")


def extract_usage_from_response(response: Any) -> Dict[str, int]:
    """Extract token usage from OpenAI API response."""
    usage = {
        "input_tokens": 0,
        "cached_tokens": 0,
        "output_tokens": 0,
    }
    
    # Try to get usage from response
    if hasattr(response, 'usage'):
        resp_usage = response.usage
        if hasattr(resp_usage, 'input_tokens'):
            usage["input_tokens"] = resp_usage.input_tokens or 0
        if hasattr(resp_usage, 'input_tokens_details'):
            details = resp_usage.input_tokens_details
            if hasattr(details, 'cached_tokens'):
                usage["cached_tokens"] = details.cached_tokens or 0
        if hasattr(resp_usage, 'output_tokens'):
            usage["output_tokens"] = resp_usage.output_tokens or 0
    
    return usage
