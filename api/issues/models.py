"""Constants and helpers for the issue automation workflow."""

from __future__ import annotations

import hashlib
import json
import re
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional


ISSUE_TYPES = {"bug", "feature", "chore"}
LEGACY_STATUSES = {"open", "in_progress", "resolved", "wontfix", "duplicate"}
WORKFLOW_STATUSES = {
    "new",
    "triaged",
    "approval_pending",
    "approved",
    "implementing",
    "review_pending",
    "pr_open",
    "merge_pending",
    "done",
    "rejected",
    "failed",
    "needs_info",
    "duplicate",
    "cancel_requested",
    "cancelled",
}
ALL_STATUSES = LEGACY_STATUSES | WORKFLOW_STATUSES

OWNER_ONLY_ACTIONS = {
    "plan",
    "approve_plan",
    "reject_plan",
    "run",
    "cancel",
}


def utc_now() -> str:
    return datetime.now(UTC).replace(tzinfo=None).isoformat()


def normalize_issue_type(value: Optional[str], description: str = "") -> str:
    raw = (value or "").strip().lower()
    if raw in ISSUE_TYPES:
        return raw

    lower_desc = description.strip().lower()
    if lower_desc.startswith(("feature request", "feature:", "request:", "suggestion:")):
        return "feature"
    if lower_desc.startswith(("chore:", "maintenance:")):
        return "chore"
    return "bug"


def title_from_description(description: str, limit: int = 80) -> str:
    clean = " ".join(description.strip().split())
    if not clean:
        return "Untitled issue"
    if len(clean) <= limit:
        return clean
    return clean[: limit - 3].rstrip() + "..."


def slugify(value: str, fallback: str = "issue", max_len: int = 48) -> str:
    slug = re.sub(r"[^a-zA-Z0-9]+", "-", value.lower()).strip("-")
    if not slug:
        slug = fallback
    return slug[:max_len].strip("-") or fallback


def json_dumps(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def json_loads(value: Optional[str], default: Any) -> Any:
    if value in (None, ""):
        return default
    try:
        return json.loads(value)
    except (TypeError, json.JSONDecodeError):
        return default


def compute_plan_hash(plan: Dict[str, Any]) -> str:
    canonical = {
        "summary": plan.get("summary", ""),
        "issue_type": plan.get("issue_type", ""),
        "requested_change": plan.get("requested_change", ""),
        "affected_paths": sorted(plan.get("affected_paths") or []),
        "risk_level": plan.get("risk_level", ""),
        "acceptance_criteria": plan.get("acceptance_criteria") or [],
        "test_plan": plan.get("test_plan") or [],
        "rollback_plan": plan.get("rollback_plan", ""),
        "automation_safe": bool(plan.get("automation_safe", False)),
    }
    return hashlib.sha256(json_dumps(canonical).encode("utf-8")).hexdigest()[:16]


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def coerce_str_list(value: Any) -> List[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return [value]
    if isinstance(value, Iterable):
        return [str(item) for item in value if str(item).strip()]
    return [str(value)]
