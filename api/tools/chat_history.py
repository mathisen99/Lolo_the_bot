"""
Chat history query tool implementation.

Allows the AI to query the message database for extended context beyond
the default 20-message window. Useful for summarizing conversations,
searching for specific topics, finding what users said, and getting
message statistics.
"""

import sqlite3
from contextlib import contextmanager
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Generator, List, Optional
from .base import Tool
import chromadb
import os
from openai import OpenAI


class ChatHistoryTool(Tool):
    """Tool for querying chat history from the database."""
    
    MAX_MESSAGES = 1000
    MAX_CHARS = 20000
    DEFAULT_LIMIT = 200
    DEFAULT_CONTEXT_MESSAGES = 50  # Messages around a specific point in time
    
    TIME_RANGES = {
        "last_hour": "hour",
        "last_6h": "6h",
        "last_24h": "24h",
        "today": "today",
        "last_week": "week",
        "last_month": "month",
    }
    
    def __init__(self):
        """Initialize chat history tool."""
        self.db_path = Path("data/bot.db")
        self.chroma_path = Path("data/chroma_db")
        self.collection_name = "chat_history"
        self.model_name = "text-embedding-3-small"
    
    @property
    def name(self) -> str:
        return "query_chat_history"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "query_chat_history",
            "description": """Query the chat history database for messages, events, or statistics.

Use this when:
- User asks about past conversations ("what did we talk about yesterday?")
- User wants to search for specific topics ("did anyone mention Python?")
- User asks for summaries ("summarize the last hour")
- User asks what someone said ("what did bob say about X?")
- User asks what someone said at a specific time ("what did bob talk about 3 hours ago?") - use hours_ago
- User asks how many messages someone sent ("how many messages did I send today?")
- User asks about activity ("how active was the channel today?")
- User asks about specific actions ("how many times did X try to generate images?") - fetch messages and analyze them
- User asks about IRC events ("has anyone been kicked?", "who got banned?", "did X change nick?") - use event_type
- You need more context than the recent 20 messages provided

For counting questions, use count_only=true for efficiency.
For looking at a specific point in time with context, use hours_ago (returns messages around that time).
For semantic analysis (like counting image generation attempts), fetch full messages and analyze the content yourself.
For IRC events (kicks, bans, quits, nick changes), use event_type filter.

Can fetch up to 1000 messages for full day analysis.

New: Use semantic=true to find concepts instead of exact words (e.g. "cats" finds "feline", "kittens").
New: Use event_type to filter by IRC events (KICK, BAN, QUIT, NICK, JOIN, PART, MODE, TOPIC).""",
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
                        "description": "Optional: filter messages by specific user nickname (case-insensitive)"
                    },
                    "time_range": {
                        "type": "string",
                        "enum": ["last_hour", "last_6h", "last_24h", "today", "last_week", "last_month"],
                        "description": "Time range to search. 'today' means since midnight. Default: last_24h. Ignored if hours_ago is set."
                    },
                    "hours_ago": {
                        "type": ["number", "null"],
                        "description": "Look at messages around X hours ago (e.g., 3.5 for 3.5 hours ago). Returns messages before AND after that point for context. Use this when user asks about a specific time like '2 hours ago' or 'this morning'. Overrides time_range."
                    },
                    "context_minutes": {
                        "type": ["integer", "null"],
                        "description": "When using hours_ago, how many minutes of context to include around that point (default: 30 minutes before and after, so 60 total). Max: 120."
                    },
                    "limit": {
                        "type": ["integer", "null"],
                        "description": "Max messages to return (1-1000). Default: 200 for time_range, 50 for hours_ago."
                    },
                    "count_only": {
                        "type": "boolean",
                        "description": "If true, only return message count statistics instead of message content. Efficient for 'how many messages' questions. Default: false"
                    },
                    "include_bot": {
                        "type": "boolean",
                        "description": "If true, include bot messages in results. Default: false (only human messages)"
                    },
                    "semantic": {
                        "type": "boolean",
                        "description": "Use semantic search instead of exact keyword matching. Useful for finding concepts, topics, or vaguely remembered conversations."
                    },
                    "event_type": {
                        "type": ["string", "null"],
                        "enum": [None, "KICK", "BAN", "UNBAN", "QUIT", "NICK", "JOIN", "PART", "MODE", "TOPIC", "ALL_EVENTS"],
                        "description": "Filter by IRC event type. Use 'ALL_EVENTS' to get all events (kicks, bans, quits, etc.). Leave empty for regular messages only."
                    },
                    "include_events": {
                        "type": "boolean",
                        "description": "If true, include IRC events (kicks, bans, quits, nick changes) alongside regular messages. Default: false"
                    }
                },
                "required": ["channel"],
                "additionalProperties": False
            }
        }
    
    @contextmanager
    def _get_connection(self) -> Generator[sqlite3.Connection, None, None]:
        """Get a read-only database connection as context manager."""
        db_uri = f"file:{self.db_path}?mode=ro"
        conn = sqlite3.connect(db_uri, uri=True)
        conn.row_factory = sqlite3.Row
        try:
            yield conn
        finally:
            conn.close()
    
    def _get_start_time(self, time_range: str) -> datetime:
        """Calculate start time based on time range."""
        now = datetime.now()
        
        if time_range == "today":
            return now.replace(hour=0, minute=0, second=0, microsecond=0)
        
        deltas = {
            "last_hour": timedelta(hours=1),
            "last_6h": timedelta(hours=6),
            "last_24h": timedelta(hours=24),
            "last_week": timedelta(days=7),
            "last_month": timedelta(days=30),
        }
        return now - deltas.get(time_range, timedelta(hours=24))
    
    def _format_timestamp(self, timestamp_str: str) -> str:
        """Format Go's verbose timestamp for readable output."""
        if len(timestamp_str) >= 19:
            return timestamp_str[:19]
        return timestamp_str
    
    def _build_where_clause(
        self,
        channel: str,
        start_time_str: str,
        nick: Optional[str],
        search_term: Optional[str],
        include_bot: bool,
        end_time_str: Optional[str] = None,
        event_type: Optional[str] = None,
        include_events: bool = False
    ) -> tuple[str, List[Any]]:
        """Build WHERE clause and params for queries."""
        conditions = [
            "LOWER(channel) = LOWER(?)",
            "substr(timestamp, 1, 19) >= ?"
        ]
        params: List[Any] = [channel, start_time_str]
        
        if end_time_str:
            conditions.append("substr(timestamp, 1, 19) <= ?")
            params.append(end_time_str)
        
        if not include_bot:
            conditions.append("is_bot = 0")
        
        if nick:
            conditions.append("LOWER(nick) = LOWER(?)")
            params.append(nick)
        
        if search_term and search_term.strip():
            conditions.append("LOWER(content) LIKE LOWER(?)")
            params.append(f"%{search_term}%")
        
        # Handle event_type filtering
        if event_type:
            if event_type == "ALL_EVENTS":
                # Get all events (non-null event_type)
                conditions.append("event_type IS NOT NULL AND event_type != ''")
            else:
                # Get specific event type
                conditions.append("event_type = ?")
                params.append(event_type)
        elif include_events:
            # Include both messages and events - no filter needed
            pass
        else:
            # Default: only regular messages (no events)
            conditions.append("(event_type IS NULL OR event_type = '')")
        
        return " AND ".join(conditions), params
    
    def _format_messages(
        self, 
        messages: List[sqlite3.Row], 
        total_found: int, 
        limit: int,
        context_mode: bool = False,
        target_time: Optional[datetime] = None,
        is_event_query: bool = False
    ) -> str:
        """Format messages for output, respecting character limit."""
        if not messages:
            if is_event_query:
                return "No events found matching your criteria."
            return "No messages found matching your criteria."
        
        lines = []
        char_count = 0
        truncated = False
        
        for msg in messages:
            timestamp = self._format_timestamp(msg["timestamp"])
            event_type = msg["event_type"] if "event_type" in msg.keys() and msg["event_type"] else None
            
            if event_type:
                # Format as event
                line = f"[{timestamp}] [{event_type}] {msg['content']}"
            else:
                # Format as regular message
                line = f"[{timestamp}] {msg['nick']}: {msg['content']}"
            
            line_len = len(line) + 1
            
            if char_count + line_len > self.MAX_CHARS:
                truncated = True
                break
            
            lines.append(line)
            char_count += line_len
        
        shown = len(lines)
        
        # Build header
        item_type = "events" if is_event_query else "messages"
        if context_mode and target_time:
            target_str = target_time.strftime("%Y-%m-%d %H:%M")
            header = f"{item_type.capitalize()} around {target_str} ({shown} {item_type})"
        elif truncated:
            header = f"Found {total_found} {item_type}, showing {shown} (truncated for length)"
        elif total_found > limit:
            header = f"Found {total_found} {item_type}, showing {shown} most recent"
        else:
            header = f"Found {total_found} {item_type}"
        
        if total_found > shown and not context_mode:
            header += ". Consider narrowing your search with a search_term or shorter time_range."
        
        return f"{header}:\n\n" + "\n".join(lines)
    
    def _get_stats(
        self,
        cursor: sqlite3.Cursor,
        channel: str,
        start_time_str: str,
        nick: Optional[str],
        search_term: Optional[str],
        include_bot: bool,
        time_desc: str,
        end_time_str: Optional[str] = None,
        event_type: Optional[str] = None,
        include_events: bool = False
    ) -> str:
        """Get message count statistics."""
        where_clause, params = self._build_where_clause(
            channel, start_time_str, nick, search_term, include_bot, end_time_str,
            event_type, include_events
        )
        
        cursor.execute(f"SELECT COUNT(*) FROM messages WHERE {where_clause}", params)
        total_count = cursor.fetchone()[0]
        
        # Determine what we're counting
        if event_type == "ALL_EVENTS":
            item_type = "event(s)"
        elif event_type:
            item_type = f"{event_type} event(s)"
        else:
            item_type = "message(s)"
        
        if nick:
            result = f"{nick}: {total_count} {item_type} in {channel} ({time_desc})"
        else:
            result = f"Total: {total_count} {item_type} in {channel} ({time_desc})"
        
        if search_term and search_term.strip():
            result += f" containing '{search_term}'"
        
        if not nick and total_count > 0 and not event_type:
            where_clause_top, params_top = self._build_where_clause(
                channel, start_time_str, None, search_term, include_bot, end_time_str,
                event_type, include_events
            )
            
            top_query = f"""
                SELECT LOWER(nick) as nick_lower, COUNT(*) as msg_count
                FROM messages
                WHERE {where_clause_top}
                GROUP BY nick_lower
                ORDER BY msg_count DESC
                LIMIT 10
            """
            
            cursor.execute(top_query, params_top)
            top_users = cursor.fetchall()
            
            if top_users:
                result += "\n\nTop contributors:\n"
                for i, row in enumerate(top_users, 1):
                    result += f"  {i}. {row['nick_lower']}: {row['msg_count']} messages\n"
        
        return result

    def _query_around_time(
        self,
        cursor: sqlite3.Cursor,
        channel: str,
        target_time: datetime,
        context_minutes: int,
        nick: Optional[str],
        search_term: Optional[str],
        include_bot: bool,
        limit: int,
        event_type: Optional[str] = None,
        include_events: bool = False
    ) -> tuple[List[sqlite3.Row], int, datetime]:
        """Query messages around a specific point in time."""
        start_time = target_time - timedelta(minutes=context_minutes)
        end_time = target_time + timedelta(minutes=context_minutes)
        
        start_str = start_time.strftime("%Y-%m-%d %H:%M:%S")
        end_str = end_time.strftime("%Y-%m-%d %H:%M:%S")
        
        where_clause, params = self._build_where_clause(
            channel, start_str, nick, search_term, include_bot, end_str,
            event_type, include_events
        )
        
        # Get count
        cursor.execute(f"SELECT COUNT(*) FROM messages WHERE {where_clause}", params)
        total_found = cursor.fetchone()[0]
        
        # Get messages in chronological order (include event_type in select)
        query = f"""
            SELECT timestamp, nick, content, COALESCE(event_type, '') as event_type
            FROM messages
            WHERE {where_clause}
            ORDER BY timestamp ASC
            LIMIT ?
        """
        params.append(limit)
        
        cursor.execute(query, params)
        messages = cursor.fetchall()
        
        return list(messages), total_found, target_time

    def execute(
        self,
        channel: str,
        search_term: Optional[str] = None,
        nick: Optional[str] = None,
        time_range: str = "last_24h",
        hours_ago: Optional[float] = None,
        context_minutes: Optional[int] = None,
        limit: Optional[int] = None,
        count_only: bool = False,
        include_bot: bool = False,
        semantic: bool = False,
        event_type: Optional[str] = None,
        include_events: bool = False,
        _current_channel: str = "",
        _permission_level: str = "normal",
        **kwargs
    ) -> str:
        """
        Query chat history from the database.
        
        Args:
            channel: Channel to search
            search_term: Optional keyword to search for (case-insensitive)
            nick: Optional user to filter by (case-insensitive)
            time_range: Time range to search (ignored if hours_ago is set)
            hours_ago: Look at messages around X hours ago
            context_minutes: Minutes of context around hours_ago point (default 30)
            limit: Max messages to return
            count_only: If True, return only statistics
            include_bot: If True, include bot messages
            semantic: If True, use semantic search (requires search_term)
            event_type: Filter by IRC event type (KICK, BAN, QUIT, NICK, etc.)
            include_events: If True, include events alongside messages
            _current_channel: Injected by system - the channel the user is currently in
            _permission_level: Injected by system - user's permission level
            
        Returns:
            Formatted message history, statistics, or error message
        """
        if not self.db_path.exists():
            return "Error: Database not found. No chat history available."
        
        if not channel or not channel.strip():
            return "Error: Channel is required."
        
        # Normalize channel for case-insensitive matching
        channel = channel.strip()
        
        # Access control: normal users can only query the channel they're currently in
        from api.utils.output import log_info as _log
        _log(f"[CHAT_HISTORY_ACL] permission_level='{_permission_level}', current_channel='{_current_channel}', requested_channel='{channel}'")
        if _permission_level not in ("owner", "admin") and _current_channel:
            if channel.strip().lower() != _current_channel.strip().lower():
                return f"Error: You can only view chat history for the channel you're currently in ({_current_channel}). Cross-channel history is restricted to admins."
        
        # Determine if this is an event query
        is_event_query = bool(event_type)

        # Handle Semantic Search
        if semantic:
            if not search_term:
                return "Error: search_term is required for semantic search."
            
            if not self.chroma_path.exists():
                return "Error: ChromaDB not found. Semantic search unavailable."

            try:
                api_key = os.getenv("OPENAI_API_KEY")
                if not api_key:
                    return "Error: OPENAI_API_KEY not set."

                # Connect to ChromaDB
                client = chromadb.PersistentClient(path=str(self.chroma_path))
                collection = client.get_collection(name=self.collection_name)
                openai_client = OpenAI(api_key=api_key)

                # Generate embedding
                response = openai_client.embeddings.create(
                    input=search_term,
                    model=self.model_name
                )
                query_embedding = response.data[0].embedding

                # Build filters - try case-insensitive channel matching
                # ChromaDB doesn't support LOWER(), so we match common case variants
                channel_lower = channel.lower()
                conditions = [{"$or": [
                    {"channel": channel},
                    {"channel": channel_lower},
                ]}]
                
                if nick:
                    conditions.append({"nick": nick})
                
                if not include_bot:
                    conditions.append({"is_bot": False})
                
                # Calculate time filter
                start_time = self._get_start_time(time_range)
                start_timestamp = int(start_time.timestamp())
                
                # Try to use timestamp_unix filter (for newer indexed messages)
                # Fall back to post-filtering for older messages without this field
                conditions.append({"timestamp_unix": {"$gte": start_timestamp}})
                
                where_clause = {"$and": conditions}

                # Query - fetch more results to account for time filtering
                fetch_limit = min((limit or 10) * 3, 100)
                results = collection.query(
                    query_embeddings=[query_embedding],
                    n_results=fetch_limit,
                    where=where_clause,
                    include=['documents', 'metadatas', 'distances']
                )

                # If no results with timestamp_unix filter, try without it (older data)
                if not results['documents'] or not results['documents'][0]:
                    # Remove timestamp_unix condition and post-filter
                    conditions = [c for c in conditions if 'timestamp_unix' not in str(c)]
                    where_clause = {"$and": conditions} if len(conditions) > 1 else conditions[0]
                    
                    results = collection.query(
                        query_embeddings=[query_embedding],
                        n_results=fetch_limit,
                        where=where_clause,
                        include=['documents', 'metadatas', 'distances']
                    )
                    
                    # Post-filter by timestamp string
                    if results['documents'] and results['documents'][0]:
                        start_time_str = start_time.strftime("%Y-%m-%d %H:%M:%S")
                        filtered_docs = []
                        filtered_metas = []
                        
                        for i, doc in enumerate(results['documents'][0]):
                            meta = results['metadatas'][0][i]
                            ts = meta.get('timestamp', '')[:19]
                            if ts >= start_time_str:
                                filtered_docs.append(doc)
                                filtered_metas.append(meta)
                        
                        results['documents'][0] = filtered_docs
                        results['metadatas'][0] = filtered_metas

                if not results['documents'] or not results['documents'][0]:
                    return f"No relevant messages found in {time_range.replace('_', ' ')}."

                # Format results (limit to requested amount)
                lines = []
                time_desc = time_range.replace('_', ' ')
                result_limit = limit or 10
                
                docs_to_show = results['documents'][0][:result_limit]
                metas_to_show = results['metadatas'][0][:result_limit]
                
                lines.append(f"Semantic Search Results for '{search_term}' ({time_desc}, {len(docs_to_show)} results):\n")
                
                for i, doc in enumerate(docs_to_show):
                    meta = metas_to_show[i]
                    # Format timestamp
                    ts = meta.get('timestamp', 'Unknown')
                    if len(ts) >= 19: ts = ts[:19]
                    
                    sender = meta.get('nick', 'Unknown')
                    line = f"[{ts}] {sender}: {doc}"
                    lines.append(line)

                return "\n".join(lines)

            except Exception as e:
                return f"Error executing semantic search: {e}"
        
        # Handle hours_ago mode (specific point in time with context)
        if hours_ago is not None:
            if hours_ago < 0:
                return "Error: hours_ago must be positive."
            if hours_ago > 24 * 30:  # Max 30 days back
                return "Error: hours_ago cannot exceed 720 (30 days)."
            
            target_time = datetime.now() - timedelta(hours=hours_ago)
            
            # Default/constrain context_minutes
            if context_minutes is None:
                context_minutes = 30
            context_minutes = max(5, min(context_minutes, 120))
            
            # Default limit for context mode
            if limit is None:
                limit = self.DEFAULT_CONTEXT_MESSAGES
            limit = max(1, min(limit, self.MAX_MESSAGES))
            
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor()
                    
                    if count_only:
                        start_time = target_time - timedelta(minutes=context_minutes)
                        end_time = target_time + timedelta(minutes=context_minutes)
                        time_desc = f"around {target_time.strftime('%H:%M')} (±{context_minutes}min)"
                        return self._get_stats(
                            cursor, channel,
                            start_time.strftime("%Y-%m-%d %H:%M:%S"),
                            nick, search_term, include_bot, time_desc,
                            end_time.strftime("%Y-%m-%d %H:%M:%S"),
                            event_type, include_events
                        )
                    
                    messages, total_found, target = self._query_around_time(
                        cursor, channel, target_time, context_minutes,
                        nick, search_term, include_bot, limit,
                        event_type, include_events
                    )
                    
                    return self._format_messages(
                        messages, total_found, limit,
                        context_mode=True, target_time=target,
                        is_event_query=is_event_query
                    )
                    
            except sqlite3.Error as e:
                return f"Error querying database: {e}"
            except Exception as e:
                return f"Error: {e}"
        
        # Standard time_range mode
        if time_range not in self.TIME_RANGES:
            valid = ", ".join(self.TIME_RANGES.keys())
            return f"Error: Invalid time_range. Must be one of: {valid}"
        
        if limit is None:
            limit = self.DEFAULT_LIMIT
        limit = max(1, min(limit, self.MAX_MESSAGES))
        
        start_time = self._get_start_time(time_range)
        start_time_str = start_time.strftime("%Y-%m-%d %H:%M:%S")
        
        try:
            with self._get_connection() as conn:
                cursor = conn.cursor()
                
                if count_only:
                    time_desc = time_range.replace("_", " ")
                    return self._get_stats(
                        cursor, channel, start_time_str, nick,
                        search_term, include_bot, time_desc,
                        None, event_type, include_events
                    )
                
                where_clause, params = self._build_where_clause(
                    channel, start_time_str, nick, search_term, include_bot,
                    None, event_type, include_events
                )
                
                cursor.execute(
                    f"SELECT COUNT(*) FROM messages WHERE {where_clause}",
                    params
                )
                total_found = cursor.fetchone()[0]
                
                # Include event_type in select
                query = f"""
                    SELECT timestamp, nick, content, COALESCE(event_type, '') as event_type
                    FROM messages
                    WHERE {where_clause}
                    ORDER BY timestamp DESC
                    LIMIT ?
                """
                params.append(limit)
                
                cursor.execute(query, params)
                messages = list(reversed(cursor.fetchall()))
                
                return self._format_messages(messages, total_found, limit, is_event_query=is_event_query)
                
        except sqlite3.Error as e:
            return f"Error querying database: {e}"
        except Exception as e:
            return f"Error: {e}"
