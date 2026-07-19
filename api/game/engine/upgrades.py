"""Pure state-schema and content-id upgrade mapping."""
from __future__ import annotations

from dataclasses import dataclass, replace
from typing import Callable

from ..models.domain import Lifecycle, SessionState, TemporaryEffect, frozen_equipment, frozen_items
from .campaign import CampaignContent, ContentUpgrade
from .invariants import known_grant_ids


@dataclass(frozen=True)
class UpgradeOutcome:
    state: SessionState
    recovery_required: bool = False
    reason: str | None = None


SchemaUpgrade = Callable[[SessionState], SessionState]


class StateUpgradeRegistry:
    """Sequential pure schema functions; no upgrade may alter the revision."""

    def __init__(self, current_version: int = 1, upgrades: dict[int, SchemaUpgrade] | None = None):
        if current_version < 1:
            raise ValueError("current schema version must be positive")
        self.current_version = current_version
        self._upgrades = dict(upgrades or {})

    def upgrade(self, state: SessionState) -> UpgradeOutcome:
        original_revision = state.state_revision
        current = state
        while current.state_schema_version < self.current_version:
            previous_version = current.state_schema_version
            upgrade = self._upgrades.get(previous_version)
            if upgrade is None:
                return _recovery(current, "state_schema_mapping_missing")
            current = upgrade(current)
            if current.state_revision != original_revision:
                raise ValueError("state schema upgrade changed the revision")
            if current.state_schema_version != previous_version + 1:
                raise ValueError("state schema upgrade did not advance exactly one version")
        if current.state_schema_version != self.current_version:
            return _recovery(current, "state_schema_incompatible")
        return UpgradeOutcome(current)


def _recovery(state: SessionState, reason: str) -> UpgradeOutcome:
    return UpgradeOutcome(replace(state, lifecycle=Lifecycle.RECOVERY_REQUIRED), True, reason)


def _mapping(entries: tuple[tuple[str, str], ...]) -> dict[str, str]:
    result = dict(entries)
    if len(result) != len(entries) or any(not source or not target for source, target in entries):
        raise ValueError("content upgrade mapping is invalid")
    return result


def _map_one(value: str, known: set[str] | frozenset[str], mappings: dict[str, str]) -> str:
    if value in known:
        return value
    replacement = mappings.get(value)
    if replacement is None or replacement not in known:
        raise KeyError(value)
    return replacement


def _map_unique(values: frozenset[str], known: frozenset[str], mappings: dict[str, str]) -> frozenset[str]:
    mapped = frozenset(_map_one(value, known, mappings) for value in values)
    if len(mapped) != len(values):
        raise KeyError("content mapping merged unique identifiers")
    return mapped


def _find_upgrade(state: SessionState, content: CampaignContent) -> ContentUpgrade | None:
    matches = [item for item in content.upgrades if item.from_version == state.content_version]
    if len(matches) > 1:
        raise ValueError("duplicate content upgrade source version")
    return matches[0] if matches else None


def upgrade_content(state: SessionState, content: CampaignContent) -> UpgradeOutcome:
    """Map every persisted content id or quarantine the untouched identifiers."""
    if state.content_version == content.version:
        return UpgradeOutcome(state)
    upgrade = _find_upgrade(state, content)
    if upgrade is None:
        return _recovery(state, "content_mapping_missing")

    locations = {location.location_id for location in content.locations}
    try:
        location_map = _mapping(upgrade.location_ids)
        item_map = _mapping(upgrade.item_ids)
        slot_map = _mapping(upgrade.equipment_slot_ids)
        ability_map = _mapping(upgrade.ability_ids)
        flag_map = _mapping(upgrade.quest_flag_ids)
        encounter_map = _mapping(upgrade.encounter_ids)
        effect_map = _mapping(upgrade.effect_ids)
        reward_map = _mapping(upgrade.reward_ids)
        profile_map = _mapping(upgrade.profile_ids)

        inventory: dict[str, int] = {}
        for item_id, quantity in state.inventory:
            mapped_item = _map_one(item_id, content.known_items, item_map)
            if mapped_item in inventory:
                raise KeyError(mapped_item)
            inventory[mapped_item] = quantity

        equipment: dict[str, str] = {}
        for slot, item_id in state.equipped:
            mapped_slot = _map_one(slot, content.equipment_slots, slot_map)
            if mapped_slot in equipment:
                raise KeyError(mapped_slot)
            equipment[mapped_slot] = _map_one(item_id, content.known_items, item_map)

        combat = state.combat
        if combat is not None:
            combat = replace(
                combat,
                encounter_id=_map_one(combat.encounter_id, content.known_encounters, encounter_map),
            )
        effects = tuple(
            TemporaryEffect(
                _map_one(effect.effect_id, content.known_effects, effect_map),
                effect.expires_on_day,
            )
            for effect in state.temporary_effects
        )
        if len({effect.effect_id for effect in effects}) != len(effects):
            raise KeyError("content mapping merged temporary effects")

        mapped = replace(
            state,
            location_id=_map_one(state.location_id, locations, location_map),
            inventory=frozen_items(inventory),
            equipped=frozen_equipment(equipment),
            abilities=_map_unique(state.abilities, content.known_abilities, ability_map),
            quest_flags=_map_unique(state.quest_flags, content.known_quest_flags, flag_map),
            claimed_rewards=_map_unique(state.claimed_rewards, known_grant_ids(content), reward_map),
            combat=combat,
            temporary_effects=effects,
            selected_content_profile=_map_one(
                state.selected_content_profile, content.available_profiles, profile_map,
            ),
            content_version=content.version,
        )
    except KeyError:
        return _recovery(state, "content_id_mapping_missing")
    if mapped.state_revision != state.state_revision:
        raise ValueError("content upgrade changed the revision")
    return UpgradeOutcome(mapped)


def upgrade_state(
    state: SessionState,
    content: CampaignContent,
    registry: StateUpgradeRegistry | None = None,
) -> UpgradeOutcome:
    schema = (registry or StateUpgradeRegistry()).upgrade(state)
    if schema.recovery_required:
        return schema
    return upgrade_content(schema.state, content)


__all__ = ["StateUpgradeRegistry", "UpgradeOutcome", "upgrade_content", "upgrade_state"]
