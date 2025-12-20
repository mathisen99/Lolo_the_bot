"""
Usage statistics tool implementation.

Allows users to query their AI usage costs and statistics.
"""

import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Generator, List, Optional
from .base import Tool


class UsageStatsTool(Tool):
    """Tool for querying AI usage statistics and costs."""
    
    # Pricing per 1M tokens (gpt-5.2)
    PRICING = {
        "gpt-5.2": {
            "input": 1.75,      # $1.75 per 1M input tokens
            "cached": 0.175,   # $0.175 per 1M cached tokens
            "output": 14.00,   # $14.00 per 1M output tokens
        }
    }
    
    TIME_RANGES = {
        "today": "today",
        "last_hour": "hour",
        "last_24h": "24h",
        "last_week": "week",
        "last_month": "month",
        "all_time": "all",
    }
    
    def __init__(self):
        """Initialize usage stats tool."""
        self.db_path = Path("data/bot.db")
    
    @property
    def name(self) -> str:
        return "usage_stats"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "usage_stats",
            "description": """Query AI usage statistics and costs.

Use this when:
- User asks how much they've spent ("how much have I spent today?")
- User asks about their usage ("how many tokens did I use?")
- User wants cost breakdown ("what's my usage this week?")
- Admin wants to see overall or per-user stats

Returns token counts and estimated costs in USD.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "nick": {
                        "type": "string",
                        "description": "The user to get stats for. Use the requesting user's nick for 'my usage' questions."
                    },
                    "time_range": {
                        "type": "string",
                        "enum": ["today", "last_hour", "last_24h", "last_week", "last_month", "all_time"],
                        "description": "Time range for statistics. Default: today"
                    },
                    "channel": {
                        "type": ["string", "null"],
                        "description": "Optional: filter by specific channel"
                    },
                    "show_top_users": {
                        "type": "boolean",
                        "description": "If true, show top users by cost instead of individual stats. Default: false"
                    }
                },
                "required": ["nick"],
                "additionalProperties": False
            }
        }
    
    @contextmanager
    def _get_connection(self) -> Generator[sqlite3.Connection, None, None]:
        """Get a read-only database connection."""
        db_uri = f"file:{self.db_path}?mode=ro"
        conn = sqlite3.connect(db_uri, uri=True)
        conn.row_factory = sqlite3.Row
        try:
            yield conn
        finally:
            conn.close()
    
    def _get_start_time(self, time_range: str) -> Optional[datetime]:
        """Calculate start time based on time range."""
        now = datetime.now()
        
        if time_range == "all_time":
            return None
        elif time_range == "today":
            return now.replace(hour=0, minute=0, second=0, microsecond=0)
        
        deltas = {
            "last_hour": timedelta(hours=1),
            "last_24h": timedelta(hours=24),
            "last_week": timedelta(days=7),
            "last_month": timedelta(days=30),
        }
        return now - deltas.get(time_range, timedelta(hours=24))
    
    def _format_cost(self, cost: float) -> str:
        """Format cost as USD string."""
        if cost < 0.01:
            return f"${cost:.4f}"
        elif cost < 1.00:
            return f"${cost:.3f}"
        else:
            return f"${cost:.2f}"
    
    def _format_tokens(self, tokens: int) -> str:
        """Format token count with K/M suffix."""
        if tokens >= 1_000_000:
            return f"{tokens / 1_000_000:.2f}M"
        elif tokens >= 1_000:
            return f"{tokens / 1_000:.1f}K"
        return str(tokens)

    def execute(
        self,
        nick: str,
        time_range: str = "today",
        channel: Optional[str] = None,
        show_top_users: bool = False,
        **kwargs
    ) -> str:
        """
        Query usage statistics.
        
        Args:
            nick: User to get stats for
            time_range: Time range for stats
            channel: Optional channel filter
            show_top_users: Show leaderboard instead of individual stats
            
        Returns:
            Formatted usage statistics
        """
        if not self.db_path.exists():
            return "Error: Database not found."
        
        if time_range not in self.TIME_RANGES:
            return f"Error: Invalid time_range. Must be one of: {', '.join(self.TIME_RANGES.keys())}"
        
        start_time = self._get_start_time(time_range)
        start_time_str = start_time.strftime("%Y-%m-%d %H:%M:%S") if start_time else None
        
        try:
            with self._get_connection() as conn:
                cursor = conn.cursor()
                
                if show_top_users:
                    return self._get_top_users(cursor, start_time_str, channel, time_range)
                else:
                    return self._get_user_stats(cursor, nick, start_time_str, channel, time_range)
                    
        except sqlite3.Error as e:
            return f"Error querying database: {e}"
        except Exception as e:
            return f"Error: {e}"
    
    def _get_user_stats(
        self,
        cursor: sqlite3.Cursor,
        nick: str,
        start_time_str: Optional[str],
        channel: Optional[str],
        time_range: str
    ) -> str:
        """Get stats for a specific user."""
        conditions = ["LOWER(nick) = LOWER(?)"]
        params: List[Any] = [nick]
        
        if start_time_str:
            conditions.append("substr(timestamp, 1, 19) >= ?")
            params.append(start_time_str)
        
        if channel:
            conditions.append("channel = ?")
            params.append(channel)
        
        where = " AND ".join(conditions)
        
        query = f"""
            SELECT 
                COUNT(*) as request_count,
                COALESCE(SUM(input_tokens), 0) as total_input,
                COALESCE(SUM(cached_tokens), 0) as total_cached,
                COALESCE(SUM(output_tokens), 0) as total_output,
                COALESCE(SUM(cost_usd), 0) as total_cost,
                COALESCE(SUM(tool_calls), 0) as total_tools
            FROM usage_tracking
            WHERE {where}
        """
        
        cursor.execute(query, params)
        row = cursor.fetchone()
        
        if not row or row['request_count'] == 0:
            return f"No usage found for {nick} ({time_range.replace('_', ' ')})."
        
        time_desc = time_range.replace("_", " ")
        result = f"Usage stats for {nick} ({time_desc}):\n"
        result += f"  Requests: {row['request_count']}\n"
        result += f"  Input tokens: {self._format_tokens(row['total_input'])}\n"
        if row['total_cached'] > 0:
            result += f"  Cached tokens: {self._format_tokens(row['total_cached'])}\n"
        result += f"  Output tokens: {self._format_tokens(row['total_output'])}\n"
        if row['total_tools'] > 0:
            result += f"  Tool calls: {row['total_tools']}\n"
        result += f"  Total cost: {self._format_cost(row['total_cost'])}"
        
        return result
    
    def _get_top_users(
        self,
        cursor: sqlite3.Cursor,
        start_time_str: Optional[str],
        channel: Optional[str],
        time_range: str
    ) -> str:
        """Get top users by cost."""
        conditions = []
        params: List[Any] = []
        
        if start_time_str:
            conditions.append("substr(timestamp, 1, 19) >= ?")
            params.append(start_time_str)
        
        if channel:
            conditions.append("channel = ?")
            params.append(channel)
        
        where = " AND ".join(conditions) if conditions else "1=1"
        
        query = f"""
            SELECT 
                LOWER(nick) as nick_lower,
                COUNT(*) as request_count,
                COALESCE(SUM(input_tokens), 0) as total_input,
                COALESCE(SUM(output_tokens), 0) as total_output,
                COALESCE(SUM(cost_usd), 0) as total_cost
            FROM usage_tracking
            WHERE {where}
            GROUP BY nick_lower
            ORDER BY total_cost DESC
            LIMIT 10
        """
        
        cursor.execute(query, params)
        rows = cursor.fetchall()
        
        if not rows:
            return f"No usage data found ({time_range.replace('_', ' ')})."
        
        # Get totals
        total_query = f"""
            SELECT 
                COUNT(*) as total_requests,
                COALESCE(SUM(cost_usd), 0) as total_cost
            FROM usage_tracking
            WHERE {where}
        """
        cursor.execute(total_query, params)
        totals = cursor.fetchone()
        
        time_desc = time_range.replace("_", " ")
        result = f"Top users by cost ({time_desc}):\n"
        
        for i, row in enumerate(rows, 1):
            result += f"  {i}. {row['nick_lower']}: {self._format_cost(row['total_cost'])} ({row['request_count']} requests)\n"
        
        result += f"\nTotal: {self._format_cost(totals['total_cost'])} across {totals['total_requests']} requests"
        
        return result
