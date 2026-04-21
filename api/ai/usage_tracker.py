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
    "gpt-5.4": {
        "input": 2.50,      # $2.50 per 1M input tokens
        "cached": 0.25,     # $0.25 per 1M cached tokens
        "output": 15.00,    # $15.00 per 1M output tokens
    },
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

# Multimodal pricing per 1M tokens for models that bill text and image tokens
# at different rates in the same request.
MULTIMODAL_PRICING = {
    "gpt-image-2": {
        "text": {
            "input": 5.00,
            "cached": 1.25,
            "output": 10.00,
        },
        "image": {
            "input": 8.00,
            "cached": 2.00,
            "output": 30.00,
        },
    },
}

# Per-call costs for native tools
WEB_SEARCH_COST = 0.01       # $10/1k calls = $0.01 each
# Note: code_interpreter removed - now using self-hosted Firecracker (free)

# GPT-5.4 long-context pricing tier:
# If input exceeds 272K tokens, apply higher rates for the full session.
HIGH_CONTEXT_TIER_MODELS = {"gpt-5.4", "gpt-5.4-pro"}
HIGH_CONTEXT_INPUT_THRESHOLD = 272_000
HIGH_CONTEXT_INPUT_MULTIPLIER = 2.0
HIGH_CONTEXT_OUTPUT_MULTIPLIER = 1.5


def calculate_cost(model: str, input_tokens: int, cached_tokens: int, output_tokens: int) -> float:
    """
    Calculate cost in USD for token usage.
    
    IMPORTANT: input_tokens is the TOTAL input, cached_tokens is a SUBSET of input_tokens.
    We charge uncached tokens at full price and cached tokens at discounted price.
    """
    pricing = PRICING.get(model, PRICING["default"])
    
    # Cached tokens are a subset of input tokens, not additional.
    # Clamp to avoid negative values from malformed usage payloads.
    cached_tokens = max(0, min(cached_tokens, input_tokens))
    uncached_tokens = max(0, input_tokens - cached_tokens)

    # GPT-5.4 pricing tier for large-context sessions.
    input_multiplier = 1.0
    output_multiplier = 1.0
    if model in HIGH_CONTEXT_TIER_MODELS and input_tokens > HIGH_CONTEXT_INPUT_THRESHOLD:
        input_multiplier = HIGH_CONTEXT_INPUT_MULTIPLIER
        output_multiplier = HIGH_CONTEXT_OUTPUT_MULTIPLIER
    
    # Cost calculation (pricing is per 1M tokens)
    uncached_cost = (uncached_tokens / 1_000_000) * pricing["input"] * input_multiplier
    cached_cost = (cached_tokens / 1_000_000) * pricing["cached"] * input_multiplier
    output_cost = (output_tokens / 1_000_000) * pricing["output"] * output_multiplier
    
    return uncached_cost + cached_cost + output_cost


def _coerce_usage_dict(usage: Any) -> Dict[str, Any]:
    """Normalize usage payloads from dicts or SDK objects into plain dicts."""
    if usage is None:
        return {}

    if isinstance(usage, dict):
        return usage

    result: Dict[str, Any] = {}
    for key in (
        "input_tokens",
        "cached_tokens",
        "output_tokens",
        "total_tokens",
        "input_tokens_details",
        "output_tokens_details",
    ):
        if hasattr(usage, key):
            value = getattr(usage, key)
            if hasattr(value, "__dict__") and not isinstance(value, dict):
                value = dict(vars(value))
            result[key] = value
    return result


def _safe_int(value: Any) -> int:
    """Coerce numeric-ish values to int."""
    try:
        return int(value or 0)
    except (TypeError, ValueError):
        return 0


def _split_cached_tokens(
    total_cached: int,
    text_input_tokens: int,
    image_input_tokens: int
) -> tuple[int, int]:
    """
    Split aggregate cached tokens between text and image inputs.

    If the API only returns a single cached token count, assign it proportionally
    based on the input token mix so cost estimation stays close to the real bill.
    """
    if total_cached <= 0:
        return (0, 0)

    if image_input_tokens <= 0:
        return (min(total_cached, text_input_tokens), 0)
    if text_input_tokens <= 0:
        return (0, min(total_cached, image_input_tokens))

    total_input = text_input_tokens + image_input_tokens
    if total_input <= 0:
        return (0, 0)

    text_cached = round(total_cached * (text_input_tokens / total_input))
    text_cached = max(0, min(text_cached, text_input_tokens, total_cached))
    image_cached = max(0, min(total_cached - text_cached, image_input_tokens))

    remainder = total_cached - text_cached - image_cached
    if remainder > 0:
        text_room = max(0, text_input_tokens - text_cached)
        add_to_text = min(remainder, text_room)
        text_cached += add_to_text
        remainder -= add_to_text

    if remainder > 0:
        image_room = max(0, image_input_tokens - image_cached)
        image_cached += min(remainder, image_room)

    return (text_cached, image_cached)


def extract_usage_from_image_result(result: Dict[str, Any]) -> Dict[str, int]:
    """Extract aggregate token counts from an Image API response payload."""
    usage = _coerce_usage_dict((result or {}).get("usage"))
    input_details = _coerce_usage_dict(usage.get("input_tokens_details"))

    cached_tokens = _safe_int(
        input_details.get("cached_tokens", usage.get("cached_tokens", 0))
    )

    return {
        "input_tokens": _safe_int(usage.get("input_tokens")),
        "cached_tokens": cached_tokens,
        "output_tokens": _safe_int(usage.get("output_tokens")),
    }


def calculate_multimodal_cost(model: str, usage: Any) -> Optional[float]:
    """
    Calculate cost for responses that include modality-specific token pricing.

    Returns None when the model doesn't need multimodal handling.
    """
    pricing = MULTIMODAL_PRICING.get(model)
    if not pricing:
        return None

    usage_dict = _coerce_usage_dict(usage)
    input_details = _coerce_usage_dict(usage_dict.get("input_tokens_details"))
    output_details = _coerce_usage_dict(usage_dict.get("output_tokens_details"))

    text_input_tokens = _safe_int(input_details.get("text_tokens"))
    image_input_tokens = _safe_int(input_details.get("image_tokens"))
    total_input_tokens = _safe_int(usage_dict.get("input_tokens"))
    if total_input_tokens > 0 and text_input_tokens + image_input_tokens == 0:
        text_input_tokens = total_input_tokens

    text_output_tokens = _safe_int(output_details.get("text_tokens"))
    image_output_tokens = _safe_int(output_details.get("image_tokens"))
    total_output_tokens = _safe_int(usage_dict.get("output_tokens"))
    if total_output_tokens > 0 and text_output_tokens + image_output_tokens == 0:
        image_output_tokens = total_output_tokens

    text_cached_tokens = _safe_int(
        input_details.get("cached_text_tokens", input_details.get("text_cached_tokens", 0))
    )
    image_cached_tokens = _safe_int(
        input_details.get("cached_image_tokens", input_details.get("image_cached_tokens", 0))
    )
    total_cached_tokens = _safe_int(
        input_details.get("cached_tokens", usage_dict.get("cached_tokens", 0))
    )

    if total_cached_tokens > 0 and text_cached_tokens + image_cached_tokens == 0:
        text_cached_tokens, image_cached_tokens = _split_cached_tokens(
            total_cached_tokens,
            text_input_tokens,
            image_input_tokens,
        )

    text_cached_tokens = max(0, min(text_cached_tokens, text_input_tokens))
    image_cached_tokens = max(0, min(image_cached_tokens, image_input_tokens))

    text_uncached_tokens = max(0, text_input_tokens - text_cached_tokens)
    image_uncached_tokens = max(0, image_input_tokens - image_cached_tokens)

    text_cost = (
        (text_uncached_tokens / 1_000_000) * pricing["text"]["input"] +
        (text_cached_tokens / 1_000_000) * pricing["text"]["cached"] +
        (text_output_tokens / 1_000_000) * pricing["text"]["output"]
    )
    image_cost = (
        (image_uncached_tokens / 1_000_000) * pricing["image"]["input"] +
        (image_cached_tokens / 1_000_000) * pricing["image"]["cached"] +
        (image_output_tokens / 1_000_000) * pricing["image"]["output"]
    )

    return text_cost + image_cost


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
    code_interpreter_calls: int = 0,
    cost_override: Optional[float] = None
) -> None:
    """Log usage to database."""
    db_path = Path("data/bot.db")
    
    if not db_path.exists():
        log_warning(f"[{request_id}] Cannot log usage: database not found")
        return
    
    if cost_override is None:
        token_cost = calculate_cost(model, input_tokens, cached_tokens, output_tokens)
        web_search_cost = web_search_calls * WEB_SEARCH_COST
        cost = token_cost + web_search_cost
    else:
        cost = cost_override
    
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
