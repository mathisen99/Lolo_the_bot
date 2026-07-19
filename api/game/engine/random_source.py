"""Replayable bounded randomness for campaign transitions."""
from __future__ import annotations

import secrets
from dataclasses import dataclass
from typing import Protocol, Sequence


@dataclass(frozen=True)
class RandomDraw:
    label: str
    upper_exclusive: int
    value: int
    table_id: str
    table_version: str

    def metadata(self) -> dict[str, str | int]:
        return {
            "label": self.label,
            "upper_exclusive": self.upper_exclusive,
            "value": self.value,
            "table_id": self.table_id,
            "table_version": self.table_version,
        }


class RandomSource(Protocol):
    def bounded_int(
        self,
        label: str,
        upper_exclusive: int,
        *,
        table_id: str,
        table_version: str,
    ) -> RandomDraw: ...


def _validate_draw_request(
    label: str,
    upper_exclusive: int,
    table_id: str,
    table_version: str,
) -> None:
    if upper_exclusive <= 0:
        raise ValueError("random bound must be positive")
    if not label or not table_id or not table_version:
        raise ValueError("random draws require a label, table id, and table version")


class SystemRandomSource:
    """Production source: every value is bounded and carries a stable label."""

    def __init__(self) -> None:
        self._random = secrets.SystemRandom()

    def bounded_int(
        self,
        label: str,
        upper_exclusive: int,
        *,
        table_id: str,
        table_version: str,
    ) -> RandomDraw:
        _validate_draw_request(label, upper_exclusive, table_id, table_version)
        return RandomDraw(
            label=label,
            upper_exclusive=upper_exclusive,
            value=self._random.randrange(upper_exclusive),
            table_id=table_id,
            table_version=table_version,
        )


class ScriptedRandomSource:
    """Strict test/replay source that verifies draw ordering and table versions."""

    def __init__(
        self,
        values: Sequence[int],
        *,
        expected: Sequence[tuple[str, str, str]] | None = None,
    ) -> None:
        self._values = tuple(values)
        self._expected = tuple(expected) if expected is not None else None
        if self._expected is not None and len(self._expected) != len(self._values):
            raise ValueError("scripted random values and expectations must have equal length")
        self._position = 0
        self._draws: list[RandomDraw] = []

    @property
    def draws(self) -> tuple[RandomDraw, ...]:
        return tuple(self._draws)

    @property
    def consumed(self) -> int:
        return self._position

    def bounded_int(
        self,
        label: str,
        upper_exclusive: int,
        *,
        table_id: str,
        table_version: str,
    ) -> RandomDraw:
        _validate_draw_request(label, upper_exclusive, table_id, table_version)
        if self._position >= len(self._values):
            raise ValueError(f"scripted random input exhausted at {label}")
        if self._expected is not None:
            if self._position >= len(self._expected):
                raise ValueError("scripted random expectation exhausted")
            expected = self._expected[self._position]
            actual = (label, table_id, table_version)
            if actual != expected:
                raise ValueError(f"random draw order mismatch: expected {expected}, got {actual}")
        value = self._values[self._position]
        if value < 0 or value >= upper_exclusive:
            raise ValueError(f"scripted random value {value} is outside [0, {upper_exclusive})")
        self._position += 1
        draw = RandomDraw(label, upper_exclusive, value, table_id, table_version)
        self._draws.append(draw)
        return draw


__all__ = ["RandomDraw", "RandomSource", "ScriptedRandomSource", "SystemRandomSource"]
