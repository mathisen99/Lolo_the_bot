"""Immutable campaign domain values used only by the deterministic engine."""
from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class Lifecycle(str, Enum):
    ACTIVE = "active"
    COMPLETED = "completed"
    FAILED = "failed"
    RECOVERY_REQUIRED = "recovery_required"


@dataclass(frozen=True)
class SessionIdentity:
    network_id: str
    kind: str
    value: str


@dataclass(frozen=True)
class CombatState:
    encounter_id: str
    encounter_version: str
    enemy_health: int
    turn: int = 1
    reward_claimed: bool = False
    consumables_used: tuple[str, ...] = ()


@dataclass(frozen=True)
class TemporaryEffect:
    effect_id: str
    expires_on_day: int


@dataclass(frozen=True)
class SessionState:
    identity: SessionIdentity
    state_revision: int
    lifecycle: Lifecycle
    location_id: str
    day: int
    countdown_remaining: int
    health: int
    max_health: int
    currency: int
    progression_level: int
    experience: int
    abilities: frozenset[str] = frozenset()
    inventory: tuple[tuple[str, int], ...] = ()
    equipped: tuple[tuple[str, str], ...] = ()
    quest_flags: frozenset[str] = frozenset()
    claimed_rewards: frozenset[str] = frozenset()
    combat: CombatState | None = None
    temporary_effects: tuple[TemporaryEffect, ...] = ()
    selected_content_profile: str = "standard"
    milestone_opt_in: bool = False
    engine_version: str = "1"
    content_version: str = "1"
    state_schema_version: int = 1

    def inventory_map(self) -> dict[str, int]:
        return dict(self.inventory)

    def equipped_map(self) -> dict[str, str]:
        return dict(self.equipped)


def frozen_items(values: Mapping[str, int] | None = None) -> tuple[tuple[str, int], ...]:
    return tuple(sorted((key, int(value)) for key, value in (values or {}).items() if value))


def frozen_equipment(values: Mapping[str, str] | None = None) -> tuple[tuple[str, str], ...]:
    return tuple(sorted((key, str(value)) for key, value in (values or {}).items()))


def state_from_mapping(value: Mapping[str, Any]) -> SessionState:
    """Decode the canonical persistence representation without mutating it."""
    identity_value = value.get("identity")
    if not isinstance(identity_value, Mapping):
        raise ValueError("session identity is missing")
    combat_value = value.get("combat")
    combat = None
    if combat_value is not None:
        if not isinstance(combat_value, Mapping):
            raise ValueError("combat state must be an object")
        combat = CombatState(
            encounter_id=str(combat_value["encounter_id"]),
            encounter_version=str(combat_value["encounter_version"]),
            enemy_health=int(combat_value["enemy_health"]),
            turn=int(combat_value.get("turn", 1)),
            reward_claimed=bool(combat_value.get("reward_claimed", False)),
            consumables_used=tuple(map(str, combat_value.get("consumables_used", ()))),
        )
    inventory_value = value.get("inventory", {})
    equipped_value = value.get("equipped", {})
    inventory = dict(inventory_value) if isinstance(inventory_value, Mapping) else dict(inventory_value)
    equipped = dict(equipped_value) if isinstance(equipped_value, Mapping) else dict(equipped_value)
    effects = tuple(
        TemporaryEffect(str(item["effect_id"]), int(item["expires_on_day"]))
        for item in value.get("temporary_effects", ())
    )
    return SessionState(
        identity=SessionIdentity(
            network_id=str(identity_value["network_id"]),
            kind=str(identity_value["kind"]),
            value=str(identity_value["value"]),
        ),
        state_revision=int(value["state_revision"]),
        lifecycle=Lifecycle(str(value["lifecycle"])),
        location_id=str(value["location_id"]),
        day=int(value["day"]),
        countdown_remaining=int(value["countdown_remaining"]),
        health=int(value["health"]),
        max_health=int(value["max_health"]),
        currency=int(value["currency"]),
        progression_level=int(value["progression_level"]),
        experience=int(value["experience"]),
        abilities=frozenset(map(str, value.get("abilities", ()))),
        inventory=frozen_items(inventory),
        equipped=frozen_equipment(equipped),
        quest_flags=frozenset(map(str, value.get("quest_flags", ()))),
        claimed_rewards=frozenset(map(str, value.get("claimed_rewards", ()))),
        combat=combat,
        temporary_effects=effects,
        selected_content_profile=str(value.get("selected_content_profile", "standard")),
        milestone_opt_in=bool(value.get("milestone_opt_in", False)),
        engine_version=str(value.get("engine_version", "1")),
        content_version=str(value.get("content_version", "1")),
        state_schema_version=int(value.get("state_schema_version", 1)),
    )


__all__ = [
    "CombatState", "Lifecycle", "SessionIdentity", "SessionState", "TemporaryEffect",
    "frozen_equipment", "frozen_items", "state_from_mapping",
]
