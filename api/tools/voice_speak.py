"""
Voice speak tool implementation.

Uses CosyVoice2 to generate speech using a reference voice sample.
Supports direct audio URLs or YouTube videos with time range extraction.
Uploads results to 0x0.st for sharing via IRC.
"""

import glob
import subprocess
import tempfile
import requests
from pathlib import Path
from typing import Any, Dict, Optional
from urllib.parse import urlparse
from .base import Tool
from api.utils.output import log_info, log_success, log_error, log_warning


class VoiceSpeakTool(Tool):
    """Voice speak tool using CosyVoice2 with YouTube support."""
    
    # Directory paths (relative to project root)
    COSYVOICE_DIR = Path(__file__).parent.parent.parent / "CosyVoice"
    ISOLATE_DIR = Path(__file__).parent.parent.parent / "IsolateVoice"
    
    # Upload endpoint
    UPLOAD_URL = "https://0x0.st"
    
    # Timeouts
    DOWNLOAD_TIMEOUT = 30
    YOUTUBE_TIMEOUT = 120  # YouTube download can be slow
    ISOLATE_TIMEOUT = 300  # Demucs isolation takes time
    TTS_TIMEOUT = 120
    UPLOAD_TIMEOUT = 30
    FFMPEG_TIMEOUT = 30
    
    # Max file sizes
    MAX_VOICE_SIZE = 50 * 1024 * 1024  # 50MB
    
    # User agent
    USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"
    
    def __init__(self):
        """Initialize voice speak tool."""
        self._session = None
    
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
    
    def _get_session(self) -> requests.Session:
        """Get or create a requests session."""
        if self._session is None:
            self._session = requests.Session()
            self._session.headers.update({
                "User-Agent": self.USER_AGENT,
                "Accept": "*/*",
            })
        return self._session
    
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
    
    # Max YouTube clip duration in seconds
    MAX_YOUTUBE_DURATION = 30
    
    def _is_youtube_url(self, url: str) -> bool:
        """Check if URL is a YouTube URL."""
        host = urlparse(url).netloc.lower()
        return any(yt in host for yt in ["youtube.com", "youtu.be", "youtube-nocookie.com"])
    
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
    
    def _download_voice(self, url: str, dest_path: Path) -> tuple[bool, str]:
        """Download voice sample from URL."""
        session = self._get_session()
        try:
            response = session.get(url, timeout=self.DOWNLOAD_TIMEOUT, allow_redirects=True, stream=True)
            response.raise_for_status()
            total_size = 0
            with open(dest_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    total_size += len(chunk)
                    if total_size > self.MAX_VOICE_SIZE:
                        return False, f"File too large (max {self.MAX_VOICE_SIZE // 1024 // 1024}MB)"
                    f.write(chunk)
            return True, ""
        except requests.exceptions.Timeout:
            return False, "Download timed out"
        except requests.exceptions.RequestException as e:
            return False, f"Download failed: {str(e)}"
    
    def _extract_youtube_clip(self, url: str, start: str, end: str, output_dir: Path) -> tuple[bool, str, Optional[Path]]:
        """Extract audio clip from YouTube video using getvideo.py."""
        cmd = f'source .venv/bin/activate && python getvideo.py "{url}" "{start}" "{end}" --audio-only -o "{output_dir}"'
        try:
            result = subprocess.run(
                ["bash", "-c", cmd],
                cwd=str(self.ISOLATE_DIR),
                capture_output=True,
                text=True,
                timeout=self.YOUTUBE_TIMEOUT
            )
            if result.returncode != 0:
                error = result.stderr.strip() or result.stdout.strip() or "Unknown error"
                return False, f"YouTube extraction failed: {error}", None
            
            # Find the output MP3
            mp3_files = list(output_dir.glob("*_clip.mp3"))
            if not mp3_files:
                return False, "YouTube extraction completed but no MP3 found", None
            
            return True, "", mp3_files[0]
        except subprocess.TimeoutExpired:
            return False, f"YouTube extraction timed out after {self.YOUTUBE_TIMEOUT}s", None
        except Exception as e:
            return False, f"YouTube extraction error: {str(e)}", None
    
    def _isolate_vocals(self, audio_path: Path, output_dir: Path) -> tuple[bool, str, Optional[Path]]:
        """Isolate vocals from audio using isolate_audio.py (Demucs)."""
        cmd = f'source .venv/bin/activate && python isolate_audio.py "{audio_path}" -o "{output_dir}"'
        try:
            result = subprocess.run(
                ["bash", "-c", cmd],
                cwd=str(self.ISOLATE_DIR),
                capture_output=True,
                text=True,
                timeout=self.ISOLATE_TIMEOUT
            )
            if result.returncode != 0:
                error = result.stderr.strip() or result.stdout.strip() or "Unknown error"
                return False, f"Vocal isolation failed: {error}", None
            
            # Find the vocals file
            vocals_files = list(output_dir.glob("*_vocals.*"))
            if not vocals_files:
                return False, "Vocal isolation completed but no vocals file found", None
            
            return True, "", vocals_files[0]
        except subprocess.TimeoutExpired:
            return False, f"Vocal isolation timed out after {self.ISOLATE_TIMEOUT}s", None
        except Exception as e:
            return False, f"Vocal isolation error: {str(e)}", None
    
    def _run_tts(self, voice_path: Path, text: str, output_path: Path) -> tuple[bool, str]:
        """Run CosyVoice TTS script."""
        cmd = f'source .venv/bin/activate && python tts.py -v "{voice_path}" -o "{output_path}" "{text}"'
        try:
            result = subprocess.run(
                ["bash", "-c", cmd],
                cwd=str(self.COSYVOICE_DIR),
                capture_output=True,
                text=True,
                timeout=self.TTS_TIMEOUT
            )
            if result.returncode != 0:
                error = result.stderr.strip() or result.stdout.strip() or "Unknown error"
                return False, f"TTS failed: {error}"
            if not output_path.exists():
                return False, "TTS completed but output file not found"
            return True, ""
        except subprocess.TimeoutExpired:
            return False, f"TTS timed out after {self.TTS_TIMEOUT}s"
        except Exception as e:
            return False, f"TTS error: {str(e)}"
    
    def _convert_to_mp3(self, wav_path: Path, mp3_path: Path) -> tuple[bool, str]:
        """Convert WAV to MP3 using ffmpeg."""
        try:
            result = subprocess.run(
                ["ffmpeg", "-y", "-i", str(wav_path), "-codec:a", "libmp3lame", "-qscale:a", "2", str(mp3_path)],
                capture_output=True,
                text=True,
                timeout=self.FFMPEG_TIMEOUT
            )
            if result.returncode != 0:
                return False, f"Conversion failed: {result.stderr.strip()}"
            if not mp3_path.exists():
                return False, "Conversion completed but MP3 not found"
            return True, ""
        except FileNotFoundError:
            return False, "ffmpeg not installed"
        except subprocess.TimeoutExpired:
            return False, "Conversion timed out"
        except Exception as e:
            return False, f"Conversion error: {str(e)}"
    
    def _upload_file(self, file_path: Path) -> tuple[bool, str]:
        """Upload file to 0x0.st using curl."""
        try:
            result = subprocess.run(
                ["curl", "-s", "-f", "-F", f"file=@{file_path}", self.UPLOAD_URL],
                capture_output=True,
                text=True,
                timeout=self.UPLOAD_TIMEOUT
            )
            if result.returncode != 0:
                return False, f"Upload failed: {result.stderr.strip() or 'Unknown error'}"
            url = result.stdout.strip()
            if not url.startswith("http"):
                return False, f"Unexpected response: {url[:100]}"
            return True, url
        except subprocess.TimeoutExpired:
            return False, "Upload timed out"
        except FileNotFoundError:
            return False, "curl not installed"
        except Exception as e:
            return False, f"Upload failed: {str(e)}"
    
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
        
        if has_youtube:
            if not start_time or not end_time:
                return "Error: youtube_url requires both start_time and end_time (e.g., start_time='1:00', end_time='1:15')"
            
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
        else:
            valid, error = self._validate_url(voice_url)
            if not valid:
                return f"Error: {error}"
        
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            output_wav = temp_path / "output.wav"
            output_mp3 = temp_path / "output.mp3"
            
            if has_youtube:
                # YouTube mode: extract clip -> try isolate vocals -> TTS
                log_info(f"[VOICE_SPEAK] Step 1/5: Extracting YouTube clip ({start_time} - {end_time})")
                success, error, clip_path = self._extract_youtube_clip(youtube_url, start_time, end_time, temp_path)
                if not success:
                    log_error(f"[VOICE_SPEAK] YouTube extraction failed: {error}")
                    return f"Error: {error}"
                log_success(f"[VOICE_SPEAK] YouTube clip extracted: {clip_path.name}")
                
                # Try to isolate vocals, but fall back to raw clip if it fails
                log_info("[VOICE_SPEAK] Step 2/5: Isolating vocals (Demucs)...")
                success, error, vocals_path = self._isolate_vocals(clip_path, temp_path)
                if success and vocals_path:
                    log_success(f"[VOICE_SPEAK] Vocals isolated: {vocals_path.name}")
                    voice_file = vocals_path
                else:
                    # Isolation failed - use raw clip directly
                    log_warning(f"[VOICE_SPEAK] Vocal isolation failed, using raw clip: {error}")
                    voice_file = clip_path
            else:
                # Direct mode: download -> TTS
                log_info(f"[VOICE_SPEAK] Step 1/4: Downloading voice sample...")
                url_path = urlparse(voice_url).path.lower()
                voice_ext = '.wav' if url_path.endswith('.wav') else '.mp3'
                voice_file = temp_path / f"voice{voice_ext}"
                
                success, error = self._download_voice(voice_url, voice_file)
                if not success:
                    log_error(f"[VOICE_SPEAK] Download failed: {error}")
                    return f"Error downloading voice: {error}"
                log_success(f"[VOICE_SPEAK] Voice sample downloaded")
            
            # Run TTS
            step = "3/5" if has_youtube else "2/4"
            log_info(f"[VOICE_SPEAK] Step {step}: Generating speech (CosyVoice)...")
            success, error = self._run_tts(voice_file, text, output_wav)
            if not success:
                log_error(f"[VOICE_SPEAK] TTS failed: {error}")
                return f"Error generating speech: {error}"
            log_success("[VOICE_SPEAK] Speech generation complete")
            
            # Convert to MP3
            step = "4/5" if has_youtube else "3/4"
            log_info(f"[VOICE_SPEAK] Step {step}: Converting to MP3...")
            success, error = self._convert_to_mp3(output_wav, output_mp3)
            if not success:
                log_error(f"[VOICE_SPEAK] Conversion failed: {error}")
                return f"Error converting: {error}"
            log_success("[VOICE_SPEAK] Converted to MP3")
            
            # Upload
            step = "5/5" if has_youtube else "4/4"
            log_info(f"[VOICE_SPEAK] Step {step}: Uploading to 0x0.st...")
            success, result = self._upload_file(output_mp3)
            if not success:
                log_error(f"[VOICE_SPEAK] Upload failed: {result}")
                return f"Error uploading: {result}"
            log_success(f"[VOICE_SPEAK] Upload complete: {result}")
            
            return result
