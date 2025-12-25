#!/usr/bin/env python3
"""
Unified voice synthesis tool.

Modes:
1. Direct voice URL:   voice.py -v URL "text to speak"
2. YouTube extraction: voice.py -y YOUTUBE_URL -s START -e END "text to speak"
3. Local voice file:   voice.py -f FILE "text to speak"

Examples:
    voice.py -v https://example.com/voice.mp3 "Hello world"
    voice.py -y "https://youtube.com/watch?v=xxx" -s 1:00 -e 1:15 "Hello world"
    voice.py -f voices/sample.wav "Hello world"
    
Output is uploaded to botbin.net and URL is printed.
Use --local to save locally instead.
"""

import sys
import os
import argparse
import subprocess
import tempfile
import hashlib
import shutil
import glob
import requests
from pathlib import Path
from urllib.parse import urlparse

# Setup paths
SCRIPT_DIR = Path(__file__).parent.absolute()
sys.path.append(str(SCRIPT_DIR / 'third_party/Matcha-TTS'))

# ============ SETTINGS ============
VOICES_CACHE_DIR = SCRIPT_DIR / "voices"
OUTPUT_DIR = SCRIPT_DIR / "output"
MODEL_PATH = SCRIPT_DIR / "pretrained_models/CosyVoice2-0.5B"
MAX_VOICE_DURATION = 15  # seconds
MAX_YOUTUBE_DURATION = 30  # seconds
BOTBIN_UPLOAD_URL = "https://botbin.net/upload"
# ==================================


def parse_time(t: str) -> float:
    """Parse time string to seconds. Supports ss, mm:ss, or hh:mm:ss."""
    t = t.strip()
    parts = t.split(":")
    if len(parts) == 1:
        return float(parts[0])
    elif len(parts) == 2:
        return int(parts[0]) * 60 + float(parts[1])
    elif len(parts) == 3:
        return int(parts[0]) * 3600 + int(parts[1]) * 60 + float(parts[2])
    raise ValueError(f"Invalid time format: {t}")


def format_time(sec: float) -> str:
    """Format seconds to hh:mm:ss.mmm"""
    h = int(sec // 3600)
    m = int((sec % 3600) // 60)
    s = sec - h * 3600 - m * 60
    return f"{h:02d}:{m:02d}:{s:06.3f}"


def get_audio_duration(path: Path) -> float:
    """Get duration of audio file using ffprobe."""
    cmd = ['ffprobe', '-v', 'quiet', '-show_entries', 'format=duration',
           '-of', 'default=noprint_wrappers=1:nokey=1', str(path)]
    result = subprocess.run(cmd, capture_output=True, text=True)
    return float(result.stdout.strip())


def convert_audio(input_path: Path, output_path: Path, max_duration: float = None):
    """Convert audio to mono 16kHz WAV, optionally trim."""
    cmd = ['ffmpeg', '-y', '-i', str(input_path), '-ac', '1', '-ar', '16000']
    if max_duration:
        cmd.extend(['-t', str(max_duration)])
    cmd.append(str(output_path))
    subprocess.run(cmd, capture_output=True, check=True)


def get_file_hash(path: Path) -> str:
    """Get MD5 hash of file for caching."""
    with open(path, 'rb') as f:
        return hashlib.md5(f.read()).hexdigest()[:12]


def transcribe_audio(audio_path: Path) -> str:
    """Transcribe audio using Whisper."""
    import whisper
    print("[VOICE] Transcribing voice sample...")
    model = whisper.load_model("base")
    result = model.transcribe(str(audio_path))
    transcript = result["text"].strip()
    print(f"[VOICE] Transcript: {transcript}")
    return transcript


def download_file(url: str, dest: Path, timeout: int = 30) -> bool:
    """Download file from URL."""
    print(f"[VOICE] Downloading: {url}")
    try:
        response = requests.get(url, timeout=timeout, stream=True)
        response.raise_for_status()
        with open(dest, 'wb') as f:
            for chunk in response.iter_content(chunk_size=8192):
                f.write(chunk)
        return True
    except Exception as e:
        print(f"[VOICE] Download failed: {e}")
        return False


def download_youtube_clip(url: str, start: str, end: str, output_dir: Path) -> Path:
    """Download YouTube video and extract audio clip."""
    print(f"[VOICE] Downloading YouTube clip ({start} - {end})...")
    
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        
        # Download video
        out_template = str(tmp_path / "%(title)s.%(ext)s")
        cmd = ["yt-dlp", "-f", "bv*+ba/b", "--merge-output-format", "mp4", "-o", out_template, url]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            error_msg = result.stderr.strip() or result.stdout.strip() or "Unknown yt-dlp error"
            raise RuntimeError(f"yt-dlp failed: {error_msg}")
        
        # Find downloaded file
        mp4_files = list(tmp_path.glob("*.mp4"))
        if not mp4_files:
            raise RuntimeError("yt-dlp did not produce an MP4 file")
        video_path = mp4_files[0]
        
        # Extract audio clip
        start_fmt = format_time(parse_time(start))
        end_fmt = format_time(parse_time(end))
        output_path = output_dir / f"yt_clip_{get_file_hash(video_path)}.mp3"
        
        cmd = [
            "ffmpeg", "-y", "-ss", start_fmt, "-to", end_fmt,
            "-i", str(video_path), "-vn", "-acodec", "libmp3lame", "-q:a", "2",
            str(output_path)
        ]
        subprocess.run(cmd, check=True, capture_output=True)
        
        print(f"[VOICE] YouTube clip extracted: {output_path.name}")
        return output_path


def isolate_vocals(audio_path: Path, output_dir: Path) -> Path:
    """Isolate vocals using Demucs."""
    print("[VOICE] Isolating vocals (Demucs)...")
    
    # Convert to WAV for Demucs
    wav_path = output_dir / "demucs_input.wav"
    cmd = ["ffmpeg", "-y", "-i", str(audio_path), "-acodec", "pcm_s16le", 
           "-ar", "44100", "-ac", "2", str(wav_path)]
    subprocess.run(cmd, check=True, capture_output=True)
    
    # Run Demucs
    demucs_out = output_dir / "demucs_out"
    demucs_out.mkdir(exist_ok=True)
    cmd = ["demucs", "--out", str(demucs_out), str(wav_path)]
    subprocess.run(cmd, check=True)
    
    # Find vocals
    escaped_stem = glob.escape(wav_path.stem)
    pattern = str(demucs_out / "*" / escaped_stem / "vocals.*")
    matches = glob.glob(pattern)
    
    if not matches:
        print("[VOICE] Warning: Vocal isolation failed, using original audio")
        return audio_path
    
    vocals_path = Path(matches[0])
    print(f"[VOICE] Vocals isolated: {vocals_path.name}")
    return vocals_path


def load_or_create_voice(voice_path: Path) -> tuple:
    """Load cached voice or create new one with transcription."""
    VOICES_CACHE_DIR.mkdir(exist_ok=True)
    
    voice_hash = get_file_hash(voice_path)
    cache_wav = VOICES_CACHE_DIR / f"{voice_hash}.wav"
    cache_txt = VOICES_CACHE_DIR / f"{voice_hash}.txt"
    
    # Check cache
    if cache_wav.exists() and cache_txt.exists():
        print(f"[VOICE] Using cached voice: {voice_hash}")
        with open(cache_txt, 'r') as f:
            transcript = f.read().strip()
        return cache_wav, transcript
    
    # Process new voice
    print(f"[VOICE] Processing voice: {voice_path}")
    duration = get_audio_duration(voice_path)
    print(f"[VOICE] Duration: {duration:.1f}s")
    
    trim_to = MAX_VOICE_DURATION if duration > MAX_VOICE_DURATION else None
    if trim_to:
        print(f"[VOICE] Trimming to {trim_to}s")
    
    convert_audio(voice_path, cache_wav, trim_to)
    transcript = transcribe_audio(cache_wav)
    
    with open(cache_txt, 'w') as f:
        f.write(transcript)
    
    print(f"[VOICE] Voice cached as: {voice_hash}")
    return cache_wav, transcript


def generate_speech(voice_wav: Path, transcript: str, text: str, output_path: Path):
    """Generate speech using CosyVoice2."""
    print("[VOICE] Loading CosyVoice2 model...")
    from cosyvoice.cli.cosyvoice import CosyVoice2
    from cosyvoice.utils.file_utils import load_wav
    import torchaudio
    import torch
    
    cosyvoice = CosyVoice2(str(MODEL_PATH), load_jit=False, load_trt=False, fp16=False)
    print("[VOICE] Model loaded!")
    
    prompt_speech = load_wav(str(voice_wav), 16000)
    
    print(f"[VOICE] Generating: {text[:100]}...")
    all_audio = []
    for result in cosyvoice.inference_zero_shot(text, transcript, prompt_speech, stream=False):
        all_audio.append(result['tts_speech'])
    
    final_audio = torch.cat(all_audio, dim=1)
    torchaudio.save(str(output_path), final_audio, cosyvoice.sample_rate)
    print(f"[VOICE] Generated: {output_path}")


def convert_to_mp3(wav_path: Path, mp3_path: Path):
    """Convert WAV to MP3."""
    cmd = ["ffmpeg", "-y", "-i", str(wav_path), "-codec:a", "libmp3lame", "-qscale:a", "2", str(mp3_path)]
    subprocess.run(cmd, check=True, capture_output=True)


def upload_file(file_path: Path) -> str:
    """Upload file to botbin.net."""
    import json
    print("[VOICE] Uploading to botbin.net...")
    api_key = os.environ.get("BOTBIN_API_KEY")
    if not api_key:
        raise RuntimeError("BOTBIN_API_KEY not configured")
    
    # Use -w to get HTTP status code, don't use -f (201 is valid success)
    result = subprocess.run(
        ["curl", "-s", "-X", "POST", BOTBIN_UPLOAD_URL,
         "-H", f"Authorization: Bearer {api_key}",
         "-F", f"file=@{file_path}",
         "-F", "retention=168h",
         "-w", "\n%{http_code}"],
        capture_output=True, text=True, timeout=60
    )
    if result.returncode != 0:
        raise RuntimeError(f"Upload failed: {result.stderr}")
    
    # Parse response - last line is HTTP status code
    lines = result.stdout.strip().rsplit("\n", 1)
    if len(lines) != 2:
        raise RuntimeError(f"Unexpected response format: {result.stdout}")
    
    body, status_code = lines
    if status_code not in ("200", "201"):
        raise RuntimeError(f"Upload failed with status {status_code}: {body}")
    
    # Parse JSON response
    response = json.loads(body)
    url = response.get("url")
    if not url:
        raise RuntimeError(f"No URL in response: {body}")
    print(f"[VOICE] Uploaded: {url}")
    return url


def main():
    parser = argparse.ArgumentParser(description="Voice synthesis with CosyVoice2")
    
    # Voice source (mutually exclusive)
    voice_group = parser.add_mutually_exclusive_group(required=True)
    voice_group.add_argument("-v", "--voice-url", help="Direct URL to voice sample (MP3/WAV)")
    voice_group.add_argument("-y", "--youtube", help="YouTube URL to extract voice from")
    voice_group.add_argument("-f", "--file", help="Local voice file path")
    
    # YouTube options
    parser.add_argument("-s", "--start", help="Start time for YouTube (e.g., 1:00)")
    parser.add_argument("-e", "--end", help="End time for YouTube (e.g., 1:15)")
    parser.add_argument("--no-isolate", action="store_true", help="Skip vocal isolation for YouTube")
    
    # Output options
    parser.add_argument("-o", "--output", help="Output file path (default: auto)")
    parser.add_argument("--local", action="store_true", help="Save locally instead of uploading")
    
    # Text to speak
    parser.add_argument("text", nargs="+", help="Text to speak")
    
    args = parser.parse_args()
    text = " ".join(args.text)
    
    if not text.strip():
        print("Error: No text provided")
        sys.exit(1)
    
    # Validate YouTube args
    if args.youtube:
        if not args.start or not args.end:
            print("Error: YouTube mode requires --start and --end times")
            sys.exit(1)
        try:
            start_sec = parse_time(args.start)
            end_sec = parse_time(args.end)
            if end_sec <= start_sec:
                print("Error: End time must be after start time")
                sys.exit(1)
            if end_sec - start_sec > MAX_YOUTUBE_DURATION:
                print(f"Error: Clip too long (max {MAX_YOUTUBE_DURATION}s)")
                sys.exit(1)
        except ValueError as e:
            print(f"Error: {e}")
            sys.exit(1)
    
    OUTPUT_DIR.mkdir(exist_ok=True)
    
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        
        # Get voice sample
        if args.youtube:
            # YouTube mode
            clip_path = download_youtube_clip(args.youtube, args.start, args.end, tmp_path)
            if not args.no_isolate:
                voice_path = isolate_vocals(clip_path, tmp_path)
            else:
                voice_path = clip_path
        elif args.voice_url:
            # URL mode
            ext = Path(urlparse(args.voice_url).path).suffix or '.mp3'
            voice_path = tmp_path / f"voice{ext}"
            if not download_file(args.voice_url, voice_path):
                print("Error: Failed to download voice sample")
                sys.exit(1)
        else:
            # Local file mode
            voice_path = Path(args.file)
            if not voice_path.exists():
                print(f"Error: File not found: {voice_path}")
                sys.exit(1)
        
        # Process voice and generate speech
        voice_wav, transcript = load_or_create_voice(voice_path)
        
        output_wav = tmp_path / "output.wav"
        generate_speech(voice_wav, transcript, text, output_wav)
        
        # Convert to MP3
        output_mp3 = tmp_path / "output.mp3"
        convert_to_mp3(output_wav, output_mp3)
        
        # Output
        if args.local:
            if args.output:
                final_path = Path(args.output)
            else:
                final_path = OUTPUT_DIR / f"voice_{hashlib.md5(text.encode()).hexdigest()[:8]}.mp3"
            shutil.copy(output_mp3, final_path)
            print(f"\n[VOICE] Saved: {final_path}")
        else:
            url = upload_file(output_mp3)
            print(f"\n{url}")


if __name__ == "__main__":
    main()
