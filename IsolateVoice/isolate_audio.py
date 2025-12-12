#!/usr/bin/env python3
import argparse
import subprocess
import sys
import shutil
import tempfile
from pathlib import Path
import glob


def check_dep(cmd_name: str):
    if shutil.which(cmd_name) is None:
        print(f"Error: '{cmd_name}' not found in PATH. Please install it first.", file=sys.stderr)
        sys.exit(1)


def run_cmd(cmd, cwd=None):
    try:
        subprocess.run(cmd, check=True, cwd=cwd)
    except subprocess.CalledProcessError as e:
        print(f"Command failed: {' '.join(cmd)}", file=sys.stderr)
        sys.exit(e.returncode)


def extract_or_convert_audio(input_path: Path, work_dir: Path) -> Path:
    """
    If input is video -> extract wav.
    If input is audio -> convert to wav (44.1kHz, stereo).
    Returns path to wav file.
    """
    audio_exts = {".mp3", ".wav", ".flac", ".m4a", ".aac", ".ogg", ".wma"}
    is_audio = input_path.suffix.lower() in audio_exts

    wav_path = work_dir / (input_path.stem + "_demucs_input.wav")

    if is_audio:
        print(f"[+] Converting audio '{input_path.name}' to WAV for Demucs...")
        cmd = [
            "ffmpeg", "-y",
            "-i", str(input_path),
            "-acodec", "pcm_s16le",
            "-ar", "44100",
            "-ac", "2",
            str(wav_path),
        ]
    else:
        print(f"[+] Extracting audio from video '{input_path.name}' to WAV for Demucs...")
        cmd = [
            "ffmpeg", "-y",
            "-i", str(input_path),
            "-vn",
            "-acodec", "pcm_s16le",
            "-ar", "44100",
            "-ac", "2",
            str(wav_path),
        ]

    run_cmd(cmd)
    return wav_path


def run_demucs(wav_path: Path, output_root: Path) -> Path:
    """
    Run Demucs on the given wav file.
    Returns path to the isolated vocals file.
    """
    print(f"[+] Running Demucs on '{wav_path.name}' (this may take a while)...")

    cmd = [
        "demucs",
        "--out", str(output_root),
        str(wav_path),
    ]
    run_cmd(cmd)

    # Find vocals.* in the output_root
    stem = wav_path.stem
    # Model dir can be 'htdemucs', 'mdx_extra_q', etc.
    # Also allow any extension: .wav, .flac, ...
    # Escape glob special characters in stem (brackets are interpreted as character classes)
    escaped_stem = glob.escape(stem)
    pattern = str(output_root / "*" / escaped_stem / "vocals.*")
    print(f"[+] Searching for Demucs vocals with pattern: {pattern}")
    matches = glob.glob(pattern)

    if not matches:
        print(
            f"Error: Could not find 'vocals.*' in Demucs output.\n"
            f"       Looked for pattern: {pattern}",
            file=sys.stderr,
        )
        for p in sorted(output_root.rglob("*")):
            print("  ", p)
        sys.exit(1)

    vocals_path = Path(matches[0])
    print(f"[+] Demucs vocals track: {vocals_path}")
    return vocals_path


def remux_video_with_vocals(input_video: Path, vocals_wav: Path, output_path: Path):
    """
    Combine original video with isolated vocals as the new audio track.
    """
    print(f"[+] Remuxing video with isolated vocals -> '{output_path.name}'")

    cmd = [
        "ffmpeg", "-y",
        "-i", str(input_video),
        "-i", str(vocals_wav),
        "-map", "0:v:0",   # take video stream from original
        "-map", "1:a:0",   # take audio from vocals wav
        "-c:v", "copy",    # do not re-encode video
        "-c:a", "aac",
        "-shortest",
        str(output_path),
    ]
    run_cmd(cmd)


def main():
    parser = argparse.ArgumentParser(
        description="Isolate voice/vocals from MP4/MP3 (and other audio/video) using ffmpeg + Demucs."
    )
    parser.add_argument("input", help="Input file (e.g. video.mp4 or audio.mp3)")
    parser.add_argument(
        "-o", "--output-dir",
        help="Directory to place output files (default: alongside input file)",
        default=None,
    )
    parser.add_argument(
        "--keep-video",
        action="store_true",
        help="For video inputs: create a new MP4 with isolated vocals as the audio track.",
    )
    parser.add_argument(
        "--no-cleanup",
        action="store_true",
        help="Do not delete intermediate working directory (for debugging).",
    )

    args = parser.parse_args()

    input_path = Path(args.input).expanduser().resolve()
    if not input_path.is_file():
        print(f"Error: input file not found: {input_path}", file=sys.stderr)
        sys.exit(1)

    # Dependencies
    check_dep("ffmpeg")
    check_dep("demucs")

    # Output directory
    if args.output_dir:
        out_dir = Path(args.output_dir).expanduser().resolve()
        out_dir.mkdir(parents=True, exist_ok=True)
    else:
        out_dir = input_path.parent

    # Create working directory (in temp or under output dir)
    if args.no_cleanup:
        work_dir = out_dir / (input_path.stem + "_work")
        work_dir.mkdir(parents=True, exist_ok=True)
    else:
        work_dir = Path(tempfile.mkdtemp(prefix="voice_iso_"))

    try:
        # Step 1: extract/convert audio to wav
        wav_path = extract_or_convert_audio(input_path, work_dir)

        # Step 2: run Demucs
        demucs_out_root = work_dir / "demucs_out"
        demucs_out_root.mkdir(parents=True, exist_ok=True)

        vocals_path = run_demucs(wav_path, demucs_out_root)

        # Step 3: move vocals.* to final location with a nice name
        final_vocals = out_dir / f"{input_path.stem}_vocals{vocals_path.suffix}"
        shutil.copy2(vocals_path, final_vocals)
        print(f"[+] Isolated vocals saved to: {final_vocals}")

        # Step 4: optionally remux video
        video_exts = {".mp4", ".mkv", ".mov", ".avi", ".webm"}
        is_video = input_path.suffix.lower() in video_exts

        if args.keep_video and is_video:
            output_video = out_dir / f"{input_path.stem}_voice_only.mp4"
            remux_video_with_vocals(input_path, final_vocals, output_video)
            print(f"[+] Video with isolated vocals saved to: {output_video}")
        elif args.keep_video and not is_video:
            print("[!] --keep-video was set, but input is not a recognized video format. Skipping video remux.")

    finally:
        if not args.no_cleanup:
            shutil.rmtree(work_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
