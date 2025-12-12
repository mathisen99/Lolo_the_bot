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


def parse_time_to_seconds(t: str) -> float:
    t = t.strip()
    parts = t.split(":")
    if len(parts) == 1:
        return float(parts[0])
    elif len(parts) == 2:
        mm, ss = parts
        return int(mm) * 60 + float(ss)
    elif len(parts) == 3:
        hh, mm, ss = parts
        return int(hh) * 3600 + int(mm) * 60 + float(ss)
    else:
        raise ValueError(f"Invalid time string: {t}")


def seconds_to_timestr(sec: float) -> str:
    h = int(sec // 3600)
    m = int((sec % 3600) // 60)
    s = sec - h * 3600 - m * 60
    return f"{h:02d}:{m:02d}:{s:06.3f}"


def format_range_for_name(start_s: float, end_s: float) -> str:
    return f"{int(start_s)}s_to_{int(end_s)}s"


def download_video(url: str, work_dir: Path) -> Path:
    """
    Downloads best possible quality and forces MP4 output using yt-dlp.
    """
    print(f"[+] Downloading video from: {url}")

    # yt-dlp:
    # - best video + best audio = bv*+ba
    # - fallback = best
    # - force mp4 output container
    out_template = str(work_dir / "%(title)s.%(ext)s")
    cmd = [
        "yt-dlp",
        "-f", "bv*+ba/b",
        "--merge-output-format", "mp4",
        "-o", out_template,
        url,
    ]
    run_cmd(cmd)

    # Find the MP4 file
    files = list(work_dir.glob("*.mp4"))
    if not files:
        print("Error: yt-dlp did not produce an MP4 file.", file=sys.stderr)
        for f in work_dir.glob("*"):
            print("Found:", f)
        sys.exit(1)

    # Just pick latest if more than one
    files.sort(key=lambda p: p.stat().st_mtime, reverse=True)
    downloaded = files[0]

    print(f"[+] Downloaded MP4: {downloaded}")
    return downloaded


def extract_clip(
    input_path: Path,
    start_str: str,
    end_str: str,
    output_path: Path,
    audio_only: bool = False,
):
    print(f"[+] Extracting clip from {start_str} to {end_str}")

    if audio_only:
        cmd = [
            "ffmpeg", "-y",
            "-ss", start_str,
            "-to", end_str,
            "-i", str(input_path),
            "-vn",
            "-acodec", "libmp3lame",
            "-q:a", "2",
            str(output_path),
        ]
    else:
        # Try without re-encoding
        cmd = [
            "ffmpeg", "-y",
            "-ss", start_str,
            "-to", end_str,
            "-i", str(input_path),
            "-c", "copy",
            str(output_path),
        ]

    try:
        run_cmd(cmd)
    except Exception:
        if not audio_only:
            print("[!] Copy mode failed. Retrying with re-encode...")
            cmd = [
                "ffmpeg", "-y",
                "-ss", start_str,
                "-to", end_str,
                "-i", str(input_path),
                "-c:v", "libx264",
                "-c:a", "aac",
                "-movflags", "+faststart",
                str(output_path),
            ]
            run_cmd(cmd)

    print(f"[+] Clip saved to: {output_path}")


def main():
    parser = argparse.ArgumentParser(
        description="Download a YouTube video as MP4 using yt-dlp, and extract a time segment."
    )
    parser.add_argument("url", help="YouTube URL")
    parser.add_argument("start", help="Start time (e.g. 1:03 or 00:01:03)")
    parser.add_argument("end", help="End time (e.g. 1:43 or 00:01:43)")
    parser.add_argument(
        "-o", "--output-dir",
        default=".",
        help="Where to save the output clip"
    )
    parser.add_argument(
        "--audio-only",
        action="store_true",
        help="Extract only audio as MP3"
    )
    parser.add_argument(
        "--no-cleanup",
        action="store_true",
        help="Keep temporary directory"
    )

    args = parser.parse_args()

    check_dep("yt-dlp")
    check_dep("ffmpeg")

    # Parse start/end
    start_s = parse_time_to_seconds(args.start)
    end_s = parse_time_to_seconds(args.end)
    if end_s <= start_s:
        print("Error: end time must be greater than start time.", file=sys.stderr)
        sys.exit(1)

    start_str = seconds_to_timestr(start_s)
    end_str = seconds_to_timestr(end_s)

    # Prepare dirs
    output_dir = Path(args.output_dir).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    if args.no_cleanup:
        work_dir = output_dir / "yt_clip_work"
        work_dir.mkdir(parents=True, exist_ok=True)
    else:
        work_dir = Path(tempfile.mkdtemp(prefix="yt_clip_"))

    try:
        # Step 1: download
        downloaded_mp4 = download_video(args.url, work_dir)

        # Step 2: name output
        range_str = format_range_for_name(start_s, end_s)
        if args.audio_only:
            out_file = output_dir / f"{downloaded_mp4.stem}_{range_str}_clip.mp3"
        else:
            out_file = output_dir / f"{downloaded_mp4.stem}_{range_str}_clip.mp4"

        # Step 3: extract
        extract_clip(
            downloaded_mp4,
            start_str,
            end_str,
            out_file,
            audio_only=args.audio_only
        )

    finally:
        if not args.no_cleanup:
            shutil.rmtree(work_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
