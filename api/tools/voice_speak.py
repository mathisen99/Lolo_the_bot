"""
Voice speak tool implementation.

Uses CosyVoice2 to generate speech using a reference voice sample.
Supports direct audio URLs or YouTube videos with time range extraction.
Uploads results to botbin.net for sharing via IRC.
"""

import subprocess
from pathlib import Path
from typing import Any, Dict, Optional
from urllib.parse import urlparse
from .base import Tool
from api.utils.output import log_info, log_success, log_error


class VoiceSpeakTool(Tool):
    """Voice speak tool using CosyVoice2 with YouTube support."""
    
    # Directory paths
    COSYVOICE_DIR = Path(__file__).parent.parent.parent / "CosyVoice"
    
    # Timeouts
    VOICE_TIMEOUT = 300  # 5 minutes for full pipeline
    
    # Max YouTube clip duration in seconds
    MAX_YOUTUBE_DURATION = 30
    
    def __init__(self):
        """Initialize voice speak tool."""
        pass
    
    @property
    def name(self) -> str:
        return "voice_speak"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "voice_speak",
            "description": """Generate speech using a reference voice sample. Two modes:

1. DIRECT AUDIO: Provide voice_url to a direct audio file (MP3, WAV)
2. YOUTUBE: Provide youtube_url + start_time + end_time to extract voice from a video

For YouTube mode, the tool will:
- Download the video segment
- Isolate vocals (remove music/background)
- Use the isolated voice as reference

Time format: "1:30" (mm:ss) or "0:01:30" (hh:mm:ss)

Examples:
- Direct: voice_url="https://example.com/voice.mp3", text="Hello world"
- YouTube: youtube_url="https://youtube.com/watch?v=xxx", start_time="1:00", end_time="1:15", text="Hello world"

Voice samples work best with 5-15 seconds of clear speech.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "voice_url": {
                        "type": ["string", "null"],
                        "description": "Direct URL to audio file (MP3, WAV). Use this OR youtube_url, not both."
                    },
                    "youtube_url": {
                        "type": ["string", "null"],
                        "description": "YouTube video URL to extract voice from."
                    },
                    "start_time": {
                        "type": ["string", "null"],
                        "description": "Start time for YouTube clip (e.g., '1:00' or '0:01:00'). Required with youtube_url."
                    },
                    "end_time": {
                        "type": ["string", "null"],
                        "description": "End time for YouTube clip (e.g., '1:15' or '0:01:15'). Required with youtube_url."
                    },
                    "text": {
                        "type": "string",
                        "description": "The text to speak using the reference voice (1-2 paragraphs max)."
                    }
                },
                "required": ["text"],
                "additionalProperties": False
            }
        }
    
    def _validate_url(self, url: str) -> tuple[bool, str]:
        """Validate URL is safe to fetch."""
        try:
            parsed = urlparse(url)
            if parsed.scheme not in ("http", "https"):
                return False, "URL must start with http:// or https://"
            if not parsed.netloc:
                return False, "Invalid URL: no host found"
            host = parsed.netloc.split(":")[0].lower()
            blocked = ["localhost", "127.0.0.1", "0.0.0.0", "::1", "10.", "192.168.", "172.16."]
            for b in blocked:
                if host.startswith(b) or host == b.rstrip("."):
                    return False, "Cannot fetch local/private URLs"
            return True, ""
        except Exception as e:
            return False, f"Invalid URL: {str(e)}"
    
    def _parse_time_to_seconds(self, t: str) -> float:
        """Parse time string to seconds. Supports mm:ss or hh:mm:ss."""
        t = t.strip()
        parts = t.split(":")
        if len(parts) == 1:
            return float(parts[0])
        elif len(parts) == 2:
            return int(parts[0]) * 60 + float(parts[1])
        elif len(parts) == 3:
            return int(parts[0]) * 3600 + int(parts[1]) * 60 + float(parts[2])
        else:
            raise ValueError(f"Invalid time format: {t}")
    
    def execute(
        self,
        text: str,
        voice_url: Optional[str] = None,
        youtube_url: Optional[str] = None,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        **kwargs
    ) -> str:
        """Generate speech using a reference voice sample."""
        # Validate text
        if not text or not text.strip():
            return "Error: No text provided"
        text = text.strip()[:2000]
        
        # Determine mode
        has_direct = voice_url and voice_url.strip()
        has_youtube = youtube_url and youtube_url.strip()
        
        if not has_direct and not has_youtube:
            return "Error: Provide either voice_url (direct audio) or youtube_url"
        
        if has_direct and has_youtube:
            return "Error: Provide either voice_url OR youtube_url, not both"
        
        # Build command
        cmd_parts = ["source", ".venv/bin/activate", "&&", "python", "voice.py"]
        
        if has_youtube:
            if not start_time or not end_time:
                return "Error: youtube_url requires both start_time and end_time"
            
            # Validate time range
            try:
                start_sec = self._parse_time_to_seconds(start_time)
                end_sec = self._parse_time_to_seconds(end_time)
            except ValueError as e:
                return f"Error: Invalid time format - {e}"
            
            if end_sec <= start_sec:
                return "Error: end_time must be after start_time"
            
            duration = end_sec - start_sec
            if duration > self.MAX_YOUTUBE_DURATION:
                return f"Error: YouTube clip too long ({duration:.0f}s). Maximum is {self.MAX_YOUTUBE_DURATION} seconds."
            
            valid, error = self._validate_url(youtube_url)
            if not valid:
                return f"Error: {error}"
            
            cmd_parts.extend(["-y", youtube_url, "-s", start_time, "-e", end_time])
            log_info(f"[VOICE_SPEAK] YouTube mode: {youtube_url} ({start_time} - {end_time})")
        else:
            valid, error = self._validate_url(voice_url)
            if not valid:
                return f"Error: {error}"
            
            cmd_parts.extend(["-v", voice_url])
            log_info(f"[VOICE_SPEAK] Direct URL mode: {voice_url}")
        
        # Add text (escape quotes)
        escaped_text = text.replace('"', '\\"')
        cmd_parts.append(f'"{escaped_text}"')
        
        cmd = " ".join(cmd_parts)
        
        log_info("[VOICE_SPEAK] Running voice synthesis pipeline...")
        
        try:
            result = subprocess.run(
                ["bash", "-c", cmd],
                cwd=str(self.COSYVOICE_DIR),
                capture_output=True,
                text=True,
                timeout=self.VOICE_TIMEOUT
            )
            
            if result.returncode != 0:
                error = result.stderr.strip() or result.stdout.strip() or "Unknown error"
                log_error(f"[VOICE_SPEAK] Failed: {error}")
                return f"Error: {error}"
            
            # The last line of stdout should be the URL
            output_lines = result.stdout.strip().split('\n')
            url = output_lines[-1].strip()
            
            if not url.startswith("http"):
                log_error(f"[VOICE_SPEAK] Unexpected output: {url[:100]}")
                return f"Error: Unexpected output from voice script"
            
            log_success(f"[VOICE_SPEAK] Success: {url}")
            return url
            
        except subprocess.TimeoutExpired:
            log_error(f"[VOICE_SPEAK] Timeout after {self.VOICE_TIMEOUT}s")
            return f"Error: Voice synthesis timed out after {self.VOICE_TIMEOUT}s"
        except Exception as e:
            log_error(f"[VOICE_SPEAK] Exception: {str(e)}")
            return f"Error: {str(e)}"
