"""
YouTube Search tool implementation.

Provides ability to search YouTube, look up videos/channels, and read comments using the YouTube Data API v3.
"""

import os
import datetime
import requests
from typing import Any, Dict, List, Optional, Union
from .base import Tool

class YouTubeSearchTool(Tool):
    """
    Tool for interacting with YouTube Data API v3.
    Supports: Search, Video Details, Channel Stats, Comment threads.
    """
    
    BASE_URL = "https://www.googleapis.com/youtube/v3"
    
    def __init__(self):
        """Initialize YouTube tool."""
        self.api_key = os.getenv("GOOGLE_API_KEY", "")
        if not self.api_key:
            # We don't raise error here, but execute will fail if key is missing
            pass
            
    @property
    def name(self) -> str:
        return "youtube_search"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": self.name,
            "description": "Interact with YouTube to search for videos/channels, get video details, or read comments. Use this when users ask to search YouTube, find a video, get stats for a channel, or see what people are saying about a video. Supported actions: 'search' (find videos), 'video_details' (get stats/desc), 'channel_details' (get stats), 'comments' (read top comments).",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["search", "video_details", "channel_details", "comments"],
                        "description": "The action to perform."
                    },
                    "query": {
                        "type": "string",
                        "description": "For 'search': The search term. For 'video_details'/'comments': The video URL or ID. For 'channel_details': The channel name or ID."
                    },
                    "max_results": {
                        "type": "integer", 
                        "description": "Number of results to return (default: 5 for search, 10 for comments)",
                        "default": 5
                    }
                },
                "required": ["action", "query"]
            }
        }

    def _get_video_id(self, query: str) -> str:
        """Extract video ID from URL or return query if it looks like an ID."""
        import re
        # Standard YouTube URL
        match = re.search(r'(?:v=|/)([\w-]{11})(?:\?|&|/|$)', query)
        if match:
            return match.group(1)
        # Short URL (youtu.be)
        match = re.search(r'youtu\.be/([\w-]{11})', query)
        if match:
            return match.group(1)
        
        # Assume it's an ID if it's 11 chars
        if len(query) == 11 and re.match(r'^[\w-]+$', query):
            return query
            
        return query

    def _get_channel_id_by_username(self, username: str) -> Optional[str]:
        """Resolve a username/handle to a channel ID."""
        # Clean handle
        username = username.strip().lstrip('@')
        
        url = f"{self.BASE_URL}/search"
        params = {
            "part": "snippet",
            "q": username,
            "type": "channel",
            "maxResults": 1,
            "key": self.api_key
        }
        
        try:
            resp = requests.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()
            
            if "items" in data and len(data["items"]) > 0:
                return data["items"][0]["snippet"]["channelId"]
            return None
        except Exception:
            return None

    def execute(self, action: str, query: str, max_results: int = 5, **kwargs) -> str:
        """Execute YouTube API request."""
        if not self.api_key:
            return "Error: GOOGLE_API_KEY not found in environment variables."

        try:
            if action == "search":
                return self._search_videos(query, max_results)
            elif action == "video_details":
                video_id = self._get_video_id(query)
                return self._get_video_details(video_id)
            elif action == "channel_details":
                return self._get_channel_details(query)
            elif action == "comments":
                video_id = self._get_video_id(query)
                return self._get_comments(video_id, max_results)
            else:
                return f"Error: Unknown action '{action}'"
        except Exception as e:
            return f"YouTube API Error: {str(e)}"

    def _search_videos(self, query: str, max_results: int) -> str:
        """Search for videos."""
        url = f"{self.BASE_URL}/search"
        params = {
            "part": "snippet",
            "q": query,
            "type": "video",
            "maxResults": min(max_results, 10),
            "key": self.api_key
        }
        
        resp = requests.get(url, params=params, timeout=10)
        resp.raise_for_status()
        data = resp.json()
        
        if not data.get("items"):
            return f"No videos found for '{query}'."

        results = []
        for item in data["items"]:
            snippet = item["snippet"]
            video_id = item["id"]["videoId"]
            title = snippet["title"]
            channel = snippet["channelTitle"]
            
            results.append(f"• {title} ({channel}) - https://youtu.be/{video_id}")
            
        return f"YouTube Search Results for '{query}':\n" + "\n".join(results)

    def _get_video_details(self, video_id: str) -> str:
        """Get details for a specific video."""
        url = f"{self.BASE_URL}/videos"
        params = {
            "part": "snippet,statistics,contentDetails",
            "id": video_id,
            "key": self.api_key
        }
        
        resp = requests.get(url, params=params, timeout=10)
        resp.raise_for_status()
        data = resp.json()
        
        if not data.get("items"):
            return f"Video not found (ID: {video_id})."
            
        video = data["items"][0]
        snippet = video["snippet"]
        stats = video["statistics"]
        
        # Parse duration (ISO 8601) - simple approximation
        duration_raw = video["contentDetails"]["duration"]
        duration = duration_raw.replace("PT", "").replace("H", "h ").replace("M", "m ").replace("S", "s").lower()
        
        # Format stats
        views = int(stats.get("viewCount", 0))
        likes = int(stats.get("likeCount", 0))
        comments = int(stats.get("commentCount", 0))
        
        return (
            f"📺 Title: {snippet['title']}\n"
            f"👤 Channel: {snippet['channelTitle']}\n"
            f"⏱️ Duration: {duration}\n"
            f"👁️ Views: {views:,}\n"
            f"👍 Likes: {likes:,}\n"
            f"📝 Published: {snippet['publishedAt'][:10]}\n"
            f"🔗 URL: https://youtu.be/{video_id}\n\n"
            f"Description: {snippet['description'][:200]}..." # Truncate desc
        )

    def _get_channel_details(self, query: str) -> str:
        """Get channel statistics."""
        # First try to treat query as ID
        channel_id = query
        
        # If it doesn't look like an ID (usually start with UC), try search
        if not query.startswith("UC"):
            resolved_id = self._get_channel_id_by_username(query)
            if resolved_id:
                channel_id = resolved_id
            else:
                 return f"Could not find channel '{query}'."

        url = f"{self.BASE_URL}/channels"
        params = {
            "part": "snippet,statistics",
            "id": channel_id,
            "key": self.api_key
        }
        
        resp = requests.get(url, params=params, timeout=10)
        resp.raise_for_status()
        data = resp.json()
        
        if not data.get("items"):
            return f"Channel not found (ID: {channel_id})."
            
        channel = data["items"][0]
        snippet = channel["snippet"]
        stats = channel["statistics"]
        
        subs = int(stats.get("subscriberCount", 0))
        videos = int(stats.get("videoCount", 0))
        views = int(stats.get("viewCount", 0))
        
        return (
            f"👤 Channel: {snippet['title']}\n"
            f"👥 Subscribers: {subs:,}\n"
            f"📹 Videos: {videos:,}\n"
            f"👁️ Total Views: {views:,}\n"
            f"📝 Description: {snippet['description'][:200]}...\n"
            f"🔗 URL: https://youtube.com/channel/{channel_id}"
        )

    def _get_comments(self, video_id: str, max_results: int) -> str:
        """Get top comments for a video."""
        url = f"{self.BASE_URL}/commentThreads"
        params = {
            "part": "snippet",
            "videoId": video_id,
            "maxResults": min(max_results, 20),
            "order": "relevance", # Top comments
            "textFormat": "plainText",
            "key": self.api_key
        }
        
        try:
            resp = requests.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()
        except Exception:
            # Comments might be disabled
            return f"Could not fetch comments for video {video_id} (might be disabled or private)."
        
        if not data.get("items"):
            return "No comments found."
            
        result_lines = [f"Top Comments for https://youtu.be/{video_id}:"]
        
        for item in data["items"]:
            comment = item["snippet"]["topLevelComment"]["snippet"]
            author = comment["authorDisplayName"]
            text = comment["textDisplay"].replace("\n", " ")
            if len(text) > 150:
                text = text[:150] + "..."
            
            likes = comment.get("likeCount", 0)
            result_lines.append(f"- {author} ({likes} likes): {text}")
            
        return "\n".join(result_lines)
