"""Privacy-safe structured game events and aggregate in-process metrics."""
from __future__ import annotations

import hashlib
import json
import logging
import re
from collections import Counter
from dataclasses import asdict, dataclass
from threading import Lock
from typing import Protocol
from uuid import UUID

_SAFE_CATEGORY = re.compile(r"^[a-z0-9][a-z0-9_.:-]{0,63}$")


def safe_session_reference(network_id: str, identity_kind: str, identity_value: str) -> str:
    """Return a non-reversible routine-log reference, never the identity itself."""
    digest = hashlib.sha256(
        f"{network_id}\x1f{identity_kind}\x1f{identity_value}".encode("utf-8")
    ).hexdigest()
    return f"session-{digest[:16]}"


def _category(value: str | None, fallback: str) -> str:
    return value if value and _SAFE_CATEGORY.fullmatch(value) else fallback


@dataclass(frozen=True)
class GameEvent:
    """The exhaustive allowlist of fields permitted in routine game telemetry."""

    request_id: str
    network: str
    session_ref: str
    action_type: str
    pre_revision: int
    post_revision: int
    latency_ms: int
    result_category: str
    error_category: str
    configuration_revision: int
    content_policy_revision: int

    def as_log_record(self) -> dict[str, str | int]:
        return {"event": "game_action", **asdict(self)}


class GameObserver(Protocol):
    def observe(self, event: GameEvent) -> None: ...


class GameTelemetry:
    """Emit JSON-only allowlisted events and retain non-identifying counters."""

    def __init__(self, logger: logging.Logger | None = None) -> None:
        self._logger = logger or logging.getLogger("lolo.game")
        self._lock = Lock()
        self._counters: Counter[tuple[str, str, str]] = Counter()
        self._latency_ms = 0

    def observe(self, event: GameEvent) -> None:
        # Reconstruct through the typed allowlist so callers cannot smuggle
        # arbitrary keys into routine logs or metrics.
        record = event.as_log_record()
        with self._lock:
            self._counters[(event.action_type, event.result_category, event.error_category)] += 1
            self._latency_ms += max(0, event.latency_ms)
        payload = json.dumps(record, sort_keys=True, separators=(",", ":"))
        if event.error_category:
            self._logger.warning(payload)
        else:
            self._logger.info(payload)

    def metrics_snapshot(self) -> dict[str, object]:
        with self._lock:
            counters = {
                f"{action}|{result}|{error or 'none'}": value
                for (action, result, error), value in sorted(self._counters.items())
            }
            return {
                "game_actions_total": counters,
                "game_action_latency_ms_total": self._latency_ms,
            }


def action_event(
    *,
    request_id: UUID,
    network_id: str,
    identity_kind: str,
    identity_value: str,
    action_type: str,
    pre_revision: int,
    post_revision: int,
    latency_ms: int,
    result_category: str,
    error_category: str | None,
    configuration_revision: int,
    content_policy_revision: int,
) -> GameEvent:
    return GameEvent(
        request_id=str(request_id),
        network=_category(network_id, "invalid_network"),
        session_ref=safe_session_reference(network_id, identity_kind, identity_value),
        action_type=_category(action_type, "invalid_action"),
        pre_revision=max(0, pre_revision),
        post_revision=max(0, post_revision),
        latency_ms=max(0, latency_ms),
        result_category=_category(result_category, "unknown_result"),
        error_category=_category(error_category, "unknown_error") if error_category else "",
        configuration_revision=max(1, configuration_revision),
        content_policy_revision=max(1, content_policy_revision),
    )


__all__ = [
    "GameEvent",
    "GameObserver",
    "GameTelemetry",
    "action_event",
    "safe_session_reference",
]
