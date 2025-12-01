"""
Chat history query tool implementation.

Allows the AI to query the message database for extended context beyond
the default 20-message window. Useful for summarizing conversations,
searching for specific topics, finding what users said, and getting
message statistics.
"""

import sqlite3
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, List, Optional
from .base import Tool


class ChatHistoryTool(Tool):
    """Tool for querying chat history from the database."""
    
    # Limits - allow full day context but cap characters
    MAX_MESSAGES = 1000  # Allow fetching full day of messages
    MAX_CHARS = 50000    # ~50k chars for analysis
    
    # Time range mappings
    TIME_RANGES = {
        "last_hour": timedelta(hours=1),
        "last_6h": timedelta(hours=6),
        "last_24h": timedelta(hours=24),
        "today": timedelta(hours=24),  # Alias for last_24h
        "last_week": timedelta(days=7),
        "last_month": timedelta(days=30),
    }
    
    def __init__(self):
        """Initialize chat history tool."""
        self.db_path = Path("data/bot.db")
    
    @property
    def name(self) -> str:
        return "query_chat_history"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "query_chat_history",
            "description": """Query the chat history database for messages or statistics.

Use this when:
- User asks about past conversations ("what did we talk about yesterday?")
- User wants to search for specific topics ("did anyone mention Python?")
- User asks for summaries ("summarize the last hour")
- User asks what someone said ("what did bob say about X?")
- User asks how many messages someone sent ("how many messages did I send today?")
- User asks about activity ("how active was the channel today?")
- User asks about specific actions ("how many times did X try to generate images?") - fetch messages and analyze them
- You need more context than the recent 20 messages provided

For counting questions, use count_only=true for efficiency.
For semantic analysis (like counting image generation attempts), fetch full messages and analyze the content yourself.

Can fetch up to 1000 messages for full day analysis.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "channel": {
                        "type": "string",
                        "description": "The IRC channel to search. Use the EXACT channel name from the current conversation (e.g., '##llm-bots', '#mathizen'). Do NOT use placeholder values."
                    },
                    "search_term": {
                        "type": ["string", "null"],
                        "description": "Optional keyword or phrase to search for in message content (case-insensitive)"
                    },
                    "nick": {
                        "type": ["string", "null"],
                        "description": "Optional: filter messages by specific user nickname"
                    },
                    "time_range": {
                        "type": "string",
                        "enum": ["last_hour", "last_6h", "last_24h", "today", "last_week", "last_month"],
                        "description": "Time range to search. 'today' is alias for 'last_24h'. Default: last_24h"
                    },
                    "limit": {
                        "type": ["integer", "null"],
                        "description": "Max messages to return (1-1000). Default: 200. Use higher values for full day analysis."
                    },
                    "count_only": {
                        "type": "boolean",
                        "description": "If true, only return message count statistics instead of message content. Efficient for 'how many messages' questions. Default: false"
                    }
                },
                "required": ["channel"],
                "additionalProperties": False
            }
        }
    
    def _get_connection(self) -> sqlite3.Connection:
        """Get a read-only database connection."""
        # Use URI mode for read-only access
        db_uri = f"file:{self.db_path}?mode=ro"
        conn = sqlite3.connect(db_uri, uri=True)
        conn.row_factory = sqlite3.Row
        return conn
    
    def _format_timestamp(self, timestamp_str: str) -> str:
        """Format timestamp for readable output."""
        # Handle Go's verbose timestamp format
        # e.g. "2025-11-27 21:48:39.770514389 +0200 EET m=+3141.074466282"
        # Extract just the date and time part
        parts = timestamp_str.split()
        if len(parts) >= 2:
            date_part = parts[0]
            time_part = parts[1].split('.')[0]  # Remove nanoseconds
            return f"{date_part} {time_part}"
        return timestamp_str
    
    def _format_messages(self, messages: List[sqlite3.Row], total_found: int) -> str:
        """Format messages for output, respecting character limit."""
        if not messages:
            return "No messages found matching your criteria."
        
        lines = []
        char_count = 0
        truncated_at = None
        
        for msg in messages:
            # Format: [timestamp] nick: content
            timestamp = self._format_timestamp(msg["timestamp"])
            nick = msg["nick"]
            content = msg["content"]
            
            line = f"[{timestamp}] {nick}: {content}"
            line_len = len(line) + 1  # +1 for newline
            
            if char_count + line_len > self.MAX_CHARS:
                truncated_at = len(lines)
                break
            
            lines.append(line)
            char_count += line_len
        
        # Build header
        shown = len(lines)
        if total_found > self.MAX_MESSAGES:
            header = f"Found {total_found} messages, showing {shown} most recent"
        elif truncated_at:
            header = f"Found {total_found} messages, showing {shown} (truncated for length)"
        else:
            header = f"Found {total_found} messages"
        
        # Add note if results were limited
        if total_found > shown:
            header += ". Consider narrowing your search with a search_term or shorter time_range."
        
        return f"{header}:\n\n" + "\n".join(lines)
    
    def _get_stats(
        self,
        cursor: sqlite3.Cursor,
        channel: str,
        start_time: datetime,
        nick: Optional[str] = None,
        search_term: Optional[str] = None
    ) -> str:
        """Get message count statistics."""
        base_query = """
            SELECT COUNT(*) as count
            FROM messages
            WHERE channel = ?
            AND timestamp >= ?
        """
        params: List[Any] = [channel, start_time.strftime("%Y-%m-%d %H:%M:%S")]
        
        if nick:
            base_query += " AND LOWER(nick) = LOWER(?)"
            params.append(nick)
        
        if search_term:
            base_query += " AND content LIKE ?"
            params.append(f"%{search_term}%")
        
        cursor.execute(base_query, params)
        total_count = cursor.fetchone()[0]
        
        # Build response
        if nick:
            result = f"{nick} sent {total_count} message(s) in {channel}"
        else:
            result = f"Total: {total_count} message(s) in {channel}"
        
        if search_term:
            result += f" containing '{search_term}'"
        
        # Get top contributors if no nick filter
        if not nick and total_count > 0:
            top_query = """
                SELECT nick, COUNT(*) as msg_count
                FROM messages
                WHERE channel = ?
                AND timestamp >= ?
            """
            top_params: List[Any] = [channel, start_time.strftime("%Y-%m-%d %H:%M:%S")]
            
            if search_term:
                top_query += " AND content LIKE ?"
                top_params.append(f"%{search_term}%")
            
            top_query += " GROUP BY LOWER(nick) ORDER BY msg_count DESC LIMIT 10"
            
            cursor.execute(top_query, top_params)
            top_users = cursor.fetchall()
            
            if top_users:
                result += "\n\nTop contributors:\n"
                for i, row in enumerate(top_users, 1):
                    result += f"  {i}. {row['nick']}: {row['msg_count']} messages\n"
        
        return result

    def execute(
        self,
        channel: str,
        search_term: Optional[str] = None,
        nick: Optional[str] = None,
        time_range: str = "last_24h",
        limit: Optional[int] = None,
        count_only: bool = False,
        **kwargs
    ) -> str:
        """
        Query chat history from the database.
        
        Args:
            channel: Channel to search
            search_term: Optional keyword to search for
            nick: Optional user to filter by
            time_range: Time range to search
            limit: Max messages to return
            count_only: If True, return only statistics
            
        Returns:
            Formatted message history, statistics, or error message
        """
        # Validate database exists
        if not self.db_path.exists():
            return "Error: Database not found. No chat history available."
        
        # Validate time range
        if time_range not in self.TIME_RANGES:
            return f"Error: Invalid time_range. Must be one of: {', '.join(self.TIME_RANGES.keys())}"
        
        # Apply limit constraints
        if limit is None:
            limit = 200  # Higher default for analysis
        limit = max(1, min(limit, self.MAX_MESSAGES))
        
        # Calculate time boundary
        time_delta = self.TIME_RANGES[time_range]
        start_time = datetime.now() - time_delta
        
        try:
            conn = self._get_connection()
            cursor = conn.cursor()
            
            # Count-only mode for statistics
            if count_only:
                result = self._get_stats(cursor, channel, start_time, nick, search_term)
                conn.close()
                return result
            
            # Build query for full messages
            query = """
                SELECT timestamp, nick, content
                FROM messages
                WHERE channel = ?
                AND timestamp >= ?
            """
            params: List[Any] = [channel, start_time.strftime("%Y-%m-%d %H:%M:%S")]
            
            # Add nick filter
            if nick:
                query += " AND LOWER(nick) = LOWER(?)"
                params.append(nick)
            
            # Add search term filter
            if search_term:
                query += " AND content LIKE ?"
                params.append(f"%{search_term}%")
            
            # First get total count
            count_query = query.replace(
                "SELECT timestamp, nick, content",
                "SELECT COUNT(*)"
            )
            cursor.execute(count_query, params)
            total_found = cursor.fetchone()[0]
            
            # Now get actual messages (ordered by time, most recent last for reading order)
            query += " ORDER BY timestamp DESC LIMIT ?"
            params.append(limit)
            
            cursor.execute(query, params)
            messages = cursor.fetchall()
            
            # Reverse to chronological order for readability
            messages = list(reversed(messages))
            
            conn.close()
            
            return self._format_messages(messages, total_found)
            
        except sqlite3.Error as e:
            return f"Error querying database: {str(e)}"
        except Exception as e:
            return f"Error: {str(e)}"
