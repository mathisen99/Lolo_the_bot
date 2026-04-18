"""Botbin upload helper used by issue artifacts."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

import requests


class BotbinClient:
    API_URL = "https://botbin.net/upload"
    RETENTION = "168h"

    def __init__(self, api_key: Optional[str] = None):
        self.api_key = api_key if api_key is not None else os.environ.get("BOTBIN_API_KEY")

    def upload_file(self, path: Path, filename: Optional[str] = None) -> str:
        if not self.api_key:
            raise RuntimeError("BOTBIN_API_KEY not configured")
        filename = filename or path.name
        with path.open("rb") as f:
            response = requests.post(
                self.API_URL,
                headers={"Authorization": f"Bearer {self.api_key}"},
                files={"file": (filename, f)},
                data={"retention": self.RETENTION},
                timeout=30,
            )
        if response.status_code not in (200, 201):
            raise RuntimeError(f"botbin upload failed: {response.status_code} {response.text[:200]}")
        data = response.json()
        url = data.get("url")
        if not url:
            raise RuntimeError(f"botbin upload returned no url: {data}")
        return str(url)

