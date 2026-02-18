"""
Sora video generation tool implementation.

Generates videos from text prompts using OpenAI's Sora 2 model.
Async job: POST /videos → poll status → download MP4 → upload to botbin.
"""

import os
import time
import requests
from typing import Any, Dict, Optional
from .base import Tool
from api.utils.botbin import upload_to_botbin
from api.utils.output import log_info, log_success, log_error, log_warning


# Pricing: $0.10 per second for sora-2
SORA2_COST_PER_SECOND = 0.10


class SoraVideoTool(Tool):
    """Video generation tool using OpenAI Sora 2 API."""

    def __init__(self):
        """Initialize Sora video tool."""
        self.api_key = os.environ.get("OPENAI_API_KEY")
        self.base_url = "https://api.openai.com/v1/videos"

    @property
    def name(self) -> str:
        return "sora_video"

    def get_definition(self) -> Dict[str, Any]:
        return {
            "type": "function",
            "name": "sora_video",
            "description": (
                "Generate a short video from a text prompt using OpenAI Sora 2. "
                "Returns a botbin URL to the MP4. Videos take 1-5 minutes to render. "
                "Rate limited: 4 videos/day for normal users, unlimited for owner. "
                "Prompt tips: describe shot type, subject, action, setting, lighting. "
                "Example: 'Wide shot of a cat walking across a sunlit kitchen counter, "
                "morning light through window, slow camera pan right.'"
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "Text description of the video to generate. Be specific about shot type, subject, action, setting, and lighting."
                    },
                    "seconds": {
                        "type": "integer",
                        "description": "Video duration in seconds (4 or 8 or 12s for owner only). Default: 4. Owner can also use 12."
                    },
                    "orientation": {
                        "type": "string",
                        "enum": ["landscape", "portrait"],
                        "description": "Video orientation. landscape=1280x720, portrait=720x1280. Default: landscape"
                    }
                },
                "required": ["prompt"],
                "additionalProperties": False
            }
        }

    def execute(
        self,
        prompt: str,
        seconds: int = 4,
        orientation: str = "landscape",
        permission_level: str = "normal",
        requesting_user: str = "unknown",
        channel: str = "",
        **kwargs
    ) -> str:
        """
        Execute video generation.

        Returns:
            Botbin URL to the MP4 or error message.
        """
        if not self.api_key:
            return "Error: OPENAI_API_KEY not configured"

        # Validate seconds
        if permission_level == "owner":
            if seconds not in (4, 8, 12):
                return "Error: seconds must be 4, 8, or 12."
        else:
            if seconds not in (4, 8):
                return "Error: seconds must be 4 or 8."

        # Check rate limit
        from .video_rate_limit import check_video_rate_limit, record_video_generation
        allowed, limit_msg = check_video_rate_limit(permission_level)
        if not allowed:
            return limit_msg

        # Resolve size
        size = "1280x720" if orientation == "landscape" else "720x1280"

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

        try:
            # 1. Start render job
            log_info(f"[sora] Starting video generation: {seconds}s {size} for {requesting_user}")
            create_resp = requests.post(
                self.base_url,
                headers=headers,
                json={
                    "model": "sora-2",
                    "prompt": prompt,
                    "size": size,
                    "seconds": str(seconds),
                },
                timeout=30,
            )

            if create_resp.status_code != 200:
                return f"Error: Sora API returned {create_resp.status_code} - {create_resp.text}"

            job = create_resp.json()
            video_id = job.get("id")
            if not video_id:
                return f"Error: No video ID in response: {job}"

            log_info(f"[sora] Job created: {video_id} status={job.get('status')}")

            # 2. Poll for completion (max 6 minutes, every 15s)
            max_polls = 24
            for i in range(max_polls):
                time.sleep(15)

                status_resp = requests.get(
                    f"{self.base_url}/{video_id}",
                    headers={"Authorization": f"Bearer {self.api_key}"},
                    timeout=30,
                )

                if status_resp.status_code != 200:
                    log_warning(f"[sora] Poll error: {status_resp.status_code}")
                    continue

                status_data = status_resp.json()
                status = status_data.get("status", "unknown")
                progress = status_data.get("progress", 0)
                log_info(f"[sora] Poll {i+1}/{max_polls}: status={status} progress={progress}%")

                if status == "completed":
                    break
                elif status == "failed":
                    return f"Error: Video generation failed - {status_data.get('error', 'Unknown error')}"
            else:
                return "Error: Video generation timed out after 6 minutes."

            # 3. Download the MP4
            log_info(f"[sora] Downloading video {video_id}")
            dl_resp = requests.get(
                f"{self.base_url}/{video_id}/content",
                headers={"Authorization": f"Bearer {self.api_key}"},
                timeout=120,
            )

            if dl_resp.status_code != 200:
                return f"Error: Failed to download video - {dl_resp.status_code}"

            video_bytes = dl_resp.content
            log_success(f"[sora] Downloaded {len(video_bytes)} bytes")

            # 4. Upload to botbin
            url = upload_to_botbin(video_bytes, "sora_video.mp4")
            log_success(f"[sora] Uploaded to botbin: {url}")

            # 5. Record for rate limiting (only non-owner)
            if permission_level != "owner":
                record_video_generation()

            # 6. Calculate cost
            cost = seconds * SORA2_COST_PER_SECOND

            # 7. Log cost to usage DB
            _log_video_cost(requesting_user, channel, cost, seconds)

            # 8. Build usage counter
            from .video_rate_limit import get_remaining_video_quota, MAX_VIDEOS_PER_DAY
            if permission_level == "owner":
                usage_tag = ""
            else:
                used = MAX_VIDEOS_PER_DAY - get_remaining_video_quota()
                usage_tag = f" [{used}/{MAX_VIDEOS_PER_DAY} today]"

            return f"{url} (Sora 2, {seconds}s {orientation}){usage_tag}"

        except requests.Timeout:
            return "Error: Request timed out communicating with Sora API."
        except Exception as e:
            log_error(f"[sora] Error: {e}")
            return f"Error: {str(e)}"


def _log_video_cost(nick: str, channel: str, cost: float, seconds: int) -> None:
    """Log video generation cost to the usage tracking database."""
    import sqlite3
    from pathlib import Path
    from datetime import datetime

    db_path = Path("data/bot.db")
    if not db_path.exists():
        return

    try:
        conn = sqlite3.connect(str(db_path))
        cursor = conn.cursor()
        cursor.execute("""
            INSERT INTO usage_tracking
            (timestamp, request_id, nick, channel, model, input_tokens, cached_tokens, output_tokens, cost_usd, tool_calls, web_search_calls, code_interpreter_calls)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
            f"sora-{int(time.time())}",
            nick,
            channel,
            f"sora-2-{seconds}s",
            0, 0, 0,
            cost,
            1, 0, 0,
        ))
        conn.commit()
        conn.close()
        log_info(f"[sora] Cost logged: ${cost:.2f} for {nick}")
    except Exception as e:
        log_warning(f"[sora] Failed to log cost: {e}")
