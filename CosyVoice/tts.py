#!/usr/bin/env python3
"""
All-in-one TTS with automatic voice cloning.

Automatically:
- Converts audio to correct format
- Trims to optimal length
- Transcribes the voice sample
- Generates speech with cloned voice

Usage:
    python tts.py -v voice.wav "Text to say"
    python tts.py -v voice.mp3 "Text to say"  # any format works
    python tts.py "Text to say"               # uses default voice
"""

import sys
import os
import argparse
import subprocess
import tempfile
import hashlib

sys.path.append('third_party/Matcha-TTS')

# ============ SETTINGS ============
DEFAULT_VOICE = "samples/voice.wav"
DEFAULT_PROMPT = "Hello, this is a sample voice recording for testing and calibration. I am speaking naturally so the system can analyze tone, clarity, and rhythm."
VOICES_CACHE_DIR = "voices"
OUTPUT_DIR = "output"
MAX_DURATION = 12  # seconds
# ==================================


def get_audio_duration(path):
    """Get duration of audio file using ffprobe"""
    cmd = ['ffprobe', '-v', 'quiet', '-show_entries', 'format=duration',
           '-of', 'default=noprint_wrappers=1:nokey=1', path]
    result = subprocess.run(cmd, capture_output=True, text=True)
    return float(result.stdout.strip())


def convert_audio(input_path, output_path, max_duration=None):
    """Convert audio to mono WAV, optionally trim"""
    cmd = ['ffmpeg', '-y', '-i', input_path, '-ac', '1', '-ar', '16000']
    if max_duration:
        cmd.extend(['-t', str(max_duration)])
    cmd.append(output_path)
    subprocess.run(cmd, capture_output=True, check=True)


def get_voice_hash(path):
    """Get hash of voice file for caching"""
    with open(path, 'rb') as f:
        return hashlib.md5(f.read()).hexdigest()[:12]


def transcribe_audio(audio_path):
    """Transcribe audio using Whisper"""
    import whisper
    print("Transcribing voice sample...")
    model = whisper.load_model("base")
    result = model.transcribe(audio_path)
    transcript = result["text"].strip()
    print(f"Transcript: {transcript}")
    return transcript


def load_or_create_voice(voice_path, voices_dir):
    """Load cached voice or create new one with transcription"""
    os.makedirs(voices_dir, exist_ok=True)
    
    voice_hash = get_voice_hash(voice_path)
    cache_wav = os.path.join(voices_dir, f"{voice_hash}.wav")
    cache_txt = os.path.join(voices_dir, f"{voice_hash}.txt")
    
    # Check cache
    if os.path.exists(cache_wav) and os.path.exists(cache_txt):
        print(f"Using cached voice: {voice_hash}")
        with open(cache_txt, 'r') as f:
            transcript = f.read().strip()
        return cache_wav, transcript
    
    # Process new voice
    print(f"Processing voice: {voice_path}")
    
    # Check duration and convert
    duration = get_audio_duration(voice_path)
    print(f"Duration: {duration:.1f}s")
    
    trim_to = MAX_DURATION if duration > MAX_DURATION else None
    if trim_to:
        print(f"Trimming to {trim_to}s")
    
    convert_audio(voice_path, cache_wav, trim_to)
    
    # Transcribe
    transcript = transcribe_audio(cache_wav)
    
    # Save transcript
    with open(cache_txt, 'w') as f:
        f.write(transcript)
    
    print(f"Voice cached as: {voice_hash}")
    return cache_wav, transcript


def main():
    parser = argparse.ArgumentParser(description="TTS with automatic voice cloning")
    parser.add_argument("-v", "--voice", help="Voice sample (any audio format)")
    parser.add_argument("-o", "--output", default="tts_output.wav", help="Output filename")
    parser.add_argument("text", nargs="*", help="Text to speak")
    args = parser.parse_args()
    
    # Get text
    if args.text:
        text = " ".join(args.text)
    else:
        print("Error: No text provided")
        parser.print_help()
        sys.exit(1)
    
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    
    # Get voice
    if args.voice:
        if not os.path.exists(args.voice):
            print(f"Error: Voice file not found: {args.voice}")
            sys.exit(1)
        voice_wav, transcript = load_or_create_voice(args.voice, VOICES_CACHE_DIR)
    else:
        voice_wav = DEFAULT_VOICE
        transcript = DEFAULT_PROMPT
        if not os.path.exists(voice_wav):
            print(f"Error: Default voice not found: {voice_wav}")
            print("Provide a voice with: python tts.py -v your_voice.wav \"text\"")
            sys.exit(1)
    
    # Load model
    print("\nLoading CosyVoice2 model...")
    from cosyvoice.cli.cosyvoice import CosyVoice2
    from cosyvoice.utils.file_utils import load_wav
    import torchaudio
    
    cosyvoice = CosyVoice2('pretrained_models/CosyVoice2-0.5B', load_jit=False, load_trt=False, fp16=False)
    print("Model loaded!\n")
    
    # Load voice
    prompt_speech = load_wav(voice_wav, 16000)
    
    # Generate
    print(f"Generating: {text}")
    output_path = os.path.join(OUTPUT_DIR, args.output)
    
    all_audio = []
    for result in cosyvoice.inference_zero_shot(text, transcript, prompt_speech, stream=False):
        all_audio.append(result['tts_speech'])
    
    import torch
    final_audio = torch.cat(all_audio, dim=1)
    torchaudio.save(output_path, final_audio, cosyvoice.sample_rate)
    
    print(f"\nDone! Saved: {output_path}")
    print(f"Play: aplay {output_path}")


if __name__ == "__main__":
    main()
