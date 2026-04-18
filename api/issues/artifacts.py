"""Local and botbin artifact storage."""

from __future__ import annotations

import re
from pathlib import Path
from typing import Optional, Tuple

from .botbin import BotbinClient
from .models import sha256_file, utc_now
from .store import IssueStore


class ArtifactManager:
    DEFAULT_ROOT = Path(__file__).parent.parent.parent / "data" / "issue_artifacts"

    def __init__(self, store: IssueStore, root: Optional[Path] = None, botbin: Optional[BotbinClient] = None):
        self.store = store
        self.root = Path(root) if root else self.DEFAULT_ROOT
        self.botbin = botbin or BotbinClient()
        self.root.mkdir(parents=True, exist_ok=True)

    def _safe_name(self, value: str) -> str:
        safe = re.sub(r"[^a-zA-Z0-9_.-]+", "-", value).strip("-")
        return safe or "artifact"

    def write_text(
        self,
        issue_id: int,
        run_id: Optional[int],
        kind: str,
        content: str,
        suffix: str = ".txt",
        upload: bool = False,
        summary: str = "",
    ) -> Tuple[int, Path, Optional[str], Optional[str]]:
        issue_dir = self.root / f"issue-{issue_id}"
        if run_id is not None:
            issue_dir = issue_dir / f"run-{run_id}"
        issue_dir.mkdir(parents=True, exist_ok=True)
        timestamp = self._safe_name(utc_now().replace(":", "-"))
        filename = f"{timestamp}-{self._safe_name(kind)}{suffix}"
        path = issue_dir / filename
        path.write_text(content, encoding="utf-8")

        botbin_url = None
        upload_error = None
        if upload:
            try:
                botbin_url = self.botbin.upload_file(path, filename=filename)
            except Exception as exc:
                upload_error = str(exc)

        artifact_id = self.store.add_artifact(
            issue_id=issue_id,
            run_id=run_id,
            kind=kind,
            path=str(path),
            sha256=sha256_file(path),
            summary=summary,
            botbin_url=botbin_url,
        )
        return artifact_id, path, botbin_url, upload_error

