# CosyVoice2 TTS

Voice cloning TTS that runs locally on your GPU.

## Quick Start

```bash
cd CosyVoice
source .venv/bin/activate
python tts.py -v path/to/voice.wav "Text you want to say"
```

## Usage

```bash
# Clone any voice and generate speech
python tts.py -v voice.wav "Hello, this is my cloned voice"
python tts.py -v voice.mp3 "Any audio format works"

# Custom output filename
python tts.py -v voice.wav -o greeting.wav "Hello there!"
```

## What It Does Automatically

1. Converts audio to correct format (mono, 16kHz WAV)
2. Trims to optimal length (12 seconds max)
3. Transcribes the voice sample using Whisper
4. Caches processed voice for faster reuse
5. Generates speech with cloned voice

## Voice Sample Tips

- **Length**: 5-15 seconds works best (auto-trimmed if longer)
- **Format**: Any audio format (WAV, MP3, etc)
- **Quality**: Clear audio, minimal background noise
- **Content**: Natural speech, not singing

## Files

```
CosyVoice/
├── tts.py              # Main script
├── voices/             # Cached processed voices
├── output/             # Generated audio files
└── pretrained_models/  # AI model (~5GB)
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Output sounds wrong | Try shorter voice sample (5-10s) |
| First run slow | Normal - model loading + transcription |
| CUDA out of memory | Close other GPU apps |
