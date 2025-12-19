"""
Botbin upload utility.

Provides file upload functionality to botbin.net for sharing images, audio, and text.
"""

import os
import tempfile
import requests
from pathlib import Path
from typing import Optional


BOTBIN_UPLOAD_URL = "https://botbin.net/upload"
DEFAULT_RETENTION = "168h"  # 1 week


def upload_to_botbin(
    file_bytes: bytes,
    filename: str,
    retention: str = DEFAULT_RETENTION,
    api_key: Optional[str] = None
) -> str:
    """
    Upload file bytes to botbin.net and return URL.
    
    Args:
        file_bytes: Raw file content
        filename: Filename with extension (e.g., "image.png", "audio.mp3")
        retention: How long to keep file (e.g., "1h", "24h", "168h", "720h")
        api_key: Botbin API key (defaults to BOTBIN_API_KEY env var)
        
    Returns:
        URL to the uploaded file
        
    Raises:
        ValueError: If API key not configured or upload fails
    """
    key = api_key or os.environ.get("BOTBIN_API_KEY")
    if not key:
        raise ValueError("BOTBIN_API_KEY not configured")
    
    with tempfile.NamedTemporaryFile(suffix=Path(filename).suffix, delete=False) as tmp:
        tmp.write(file_bytes)
        tmp_path = tmp.name
    
    try:
        with open(tmp_path, "rb") as f:
            response = requests.post(
                BOTBIN_UPLOAD_URL,
                headers={"Authorization": f"Bearer {key}"},
                files={"file": (filename, f)},
                data={"retention": retention},
                timeout=60
            )
        
        if response.status_code not in (200, 201):
            raise ValueError(f"Upload failed: {response.status_code} {response.text}")
        
        # Response is JSON with url field
        result = response.json()
        url = result.get("url")
        if not url:
            raise ValueError(f"No URL in response: {result}")
        
        return url
    finally:
        os.unlink(tmp_path)


def upload_file_to_botbin(
    file_path: Path,
    retention: str = DEFAULT_RETENTION,
    api_key: Optional[str] = None
) -> str:
    """
    Upload a file from disk to botbin.net.
    
    Args:
        file_path: Path to file to upload
        retention: How long to keep file
        api_key: Botbin API key (defaults to BOTBIN_API_KEY env var)
        
    Returns:
        URL to the uploaded file
    """
    key = api_key or os.environ.get("BOTBIN_API_KEY")
    if not key:
        raise ValueError("BOTBIN_API_KEY not configured")
    
    with open(file_path, "rb") as f:
        response = requests.post(
            BOTBIN_UPLOAD_URL,
            headers={"Authorization": f"Bearer {key}"},
            files={"file": (file_path.name, f)},
            data={"retention": retention},
            timeout=60
        )
    
    if response.status_code not in (200, 201):
        raise ValueError(f"Upload failed: {response.status_code} {response.text}")
    
    # Response is JSON with url field
    result = response.json()
    url = result.get("url")
    if not url:
        raise ValueError(f"No URL in response: {result}")
    
    return url
