"""Pre/post campaign invariants shared by the engine and persistence boundary."""
from __future__ import annotations

from collections.abc import Mapping, Sequence

from ..config import CampaignConfig
from ..models.domain import Lifecycle, SessionState, state_from_mapping
from .campaign import CampaignContent


class InvariantViolation(ValueError):
    pass


def _require(condition: bool, message: str) -> None:
    if not condition:
        raise InvariantViolation(message)


def known_grant_ids(content: CampaignContent) -> frozenset[str]:
    grants = {
        outcome.grant_id
        for table in content.random_tables
        for outcome in table.outcomes
        if outcome.grant_id is not None
    }
    grants.update(investigation.grant_id for investigation in content.investigations)
    grants.update(threshold.grant_id for threshold in content.progression_thresholds)
    return frozenset(grants)


def validate_serialized_shape(value: Mapping[str, object]) -> None:
    """Reject duplicate identifiers before immutable decoding can collapse them."""
    for key in ("claimed_rewards", "quest_flags", "abilities"):
        raw = value.get(key, ())
        if isinstance(raw, Sequence) and not isinstance(raw, (str, bytes)):
            _require(len(raw) == len(set(raw)), f"{key} contains duplicate ids")
    for key in ("inventory", "equipped"):
        raw = value.get(key, {})
        if isinstance(raw, Sequence) and not isinstance(raw, (str, bytes)):
            identifiers = [item[0] for item in raw if isinstance(item, Sequence) and len(item) == 2]
            _require(len(identifiers) == len(raw), f"{key} contains an invalid entry")
            _require(len(identifiers) == len(set(identifiers)), f"{key} contains duplicate ids")


def validate_state(
    state: SessionState,
    config: CampaignConfig,
    content: CampaignContent,
    *,
    expected_revision: int | None = None,
    expected_schema_version: int = 1,
    allow_incompatible_content: bool = False,
) -> None:
    if expected_revision is not None:
        _require(state.state_revision == expected_revision, "state revision mismatch")
    _require(state.state_revision >= 0, "state revision is negative")
    _require(
        config.starting_day <= state.day <= config.starting_day + config.starting_countdown,
        "day is outside its valid range",
    )
    _require(0 <= state.countdown_remaining <= config.starting_countdown, "countdown is outside its valid range")
    _require(1 <= state.max_health <= config.maximum_health, "maximum health is outside its valid range")
    _require(0 <= state.health <= state.max_health, "health is outside its valid range")
    _require(0 <= state.currency <= config.maximum_currency, "currency is outside its valid range")
    _require(1 <= state.progression_level <= config.maximum_level, "level is outside its valid range")
    _require(0 <= state.experience <= config.maximum_experience, "experience is outside its valid range")
    _require(state.identity.kind in {"registered_user", "unregistered_nick"}, "identity kind is invalid")
    _require(bool(state.identity.network_id and state.identity.value), "identity is incomplete")

    inventory = state.inventory_map()
    equipped = state.equipped_map()
    _require(len(inventory) == len(state.inventory), "inventory contains duplicate ids")
    _require(len(equipped) == len(state.equipped), "equipment contains duplicate slots")
    _require(
        all(1 <= quantity <= config.maximum_item_quantity for quantity in inventory.values()),
        "item quantity is outside its valid range",
    )
    if state.lifecycle != Lifecycle.RECOVERY_REQUIRED and not allow_incompatible_content:
        inventory_load = sum(
            content.item(item_id).capacity_cost * quantity
            for item_id, quantity in inventory.items()
        )
        _require(inventory_load <= config.inventory_capacity, "inventory exceeds configured capacity")
    _require(
        all(inventory.get(item_id, 0) > 0 for item_id in equipped.values()),
        "equipped item is absent from inventory",
    )
    effect_ids = [effect.effect_id for effect in state.temporary_effects]
    _require(len(effect_ids) == len(set(effect_ids)), "temporary effects contain duplicate ids")
    _require(
        all(effect.expires_on_day >= state.day for effect in state.temporary_effects),
        "expired temporary effect remains active",
    )

    # Recovery-required states are lossless quarantine records. Their unknown
    # content identifiers and combat sub-state are retained for operator mapping.
    content_compatible = state.lifecycle != Lifecycle.RECOVERY_REQUIRED and not allow_incompatible_content
    if content_compatible:
        location_ids = {location.location_id for location in content.locations}
        _require(state.location_id in location_ids, "location id is unknown")
        _require(set(inventory).issubset(content.known_items), "inventory contains an unknown item")
        _require(set(equipped).issubset(content.equipment_slots), "equipment contains an unknown slot")
        _require(set(equipped.values()).issubset(content.known_items), "equipment contains an unknown item")
        _require(state.abilities.issubset(content.known_abilities), "ability id is unknown")
        _require(state.quest_flags.issubset(content.known_quest_flags), "quest flag id is unknown")
        _require(state.claimed_rewards.issubset(known_grant_ids(content)), "claimed grant id is unknown")
        _require(state.selected_content_profile in content.available_profiles, "content profile is unavailable")
        _require(state.content_version == content.version, "content version is incompatible")
        _require(state.state_schema_version == expected_schema_version, "state schema version is incompatible")
        _require(
            all(effect.effect_id in content.known_effects for effect in state.temporary_effects),
            "temporary effect id is unknown",
        )
        for implication in content.quest_implications:
            if implication.consequence_flag in state.quest_flags:
                _require(
                    implication.required_flags.issubset(state.quest_flags),
                    "quest implication is violated",
                )

    if state.lifecycle in {Lifecycle.COMPLETED, Lifecycle.FAILED}:
        _require(state.combat is None, "terminal lifecycle cannot remain in combat")
    if state.combat is not None and state.lifecycle != Lifecycle.RECOVERY_REQUIRED:
        _require(state.lifecycle == Lifecycle.ACTIVE, "combat requires an active lifecycle")
        if content_compatible:
            _require(state.combat.encounter_id in content.known_encounters, "combat encounter id is unknown")
            configured_encounter = next(
                (item for item in content.encounters if item.encounter_id == state.combat.encounter_id),
                None,
            )
            if configured_encounter is not None:
                _require(
                    state.combat.encounter_version == configured_encounter.version,
                    "combat encounter version is incompatible",
                )
                _require(
                    state.combat.enemy_health <= configured_encounter.enemy_max_health,
                    "combat enemy health exceeds its configured maximum",
                )
            _require(
                all(item_id in content.known_items for item_id in state.combat.consumables_used),
                "combat consumable id is unknown",
            )
        _require(state.combat.enemy_health > 0 and state.combat.turn >= 1, "combat values are invalid")


def validate_transition(
    before: SessionState | None,
    after: SessionState,
    config: CampaignConfig,
    content: CampaignContent,
    *,
    action_name: str,
    expected_schema_version: int = 1,
) -> None:
    validate_state(
        after,
        config,
        content,
        expected_revision=(before.state_revision + 1 if before else 1),
        expected_schema_version=expected_schema_version,
    )
    if before is not None and action_name not in {"advance", "reset"}:
        _require(after.day == before.day, "ordinary action changed the campaign day")
        _require(after.countdown_remaining == before.countdown_remaining, "ordinary action changed the countdown")
    if before is not None and action_name == "advance":
        _require(after.day == before.day + 1, "advance did not increment exactly one day")
        _require(
            after.countdown_remaining == max(0, before.countdown_remaining - 1),
            "advance did not decrement countdown exactly once",
        )


def validate_persisted_state(
    value: object,
    revision: int,
    config: CampaignConfig,
    content: CampaignContent,
    *,
    expected_schema_version: int = 1,
) -> None:
    if isinstance(value, SessionState):
        state = value
    elif isinstance(value, Mapping):
        validate_serialized_shape(value)
        state = state_from_mapping(value)
    else:
        raise InvariantViolation("persistent state must be an object")
    validate_state(
        state,
        config,
        content,
        expected_revision=revision,
        expected_schema_version=expected_schema_version,
        allow_incompatible_content=(
            state.content_version != content.version
            or state.state_schema_version != expected_schema_version
        ),
    )


__all__ = [
    "InvariantViolation", "known_grant_ids", "validate_persisted_state",
    "validate_serialized_shape", "validate_state", "validate_transition",
]
