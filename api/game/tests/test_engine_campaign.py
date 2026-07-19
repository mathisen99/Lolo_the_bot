from __future__ import annotations

import sqlite3
from dataclasses import FrozenInstanceError, replace
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

import pytest

from api.game.application import ContentSnapshot, EngineContext, GameServiceError, NormalizedAction, StoreActionRequest
from api.game.config import CampaignConfig, GameConfigSnapshot
from api.game.engine import (
    CampaignEngine,
    ContentUpgrade,
    ScriptedRandomSource,
    StateUpgradeRegistry,
    default_campaign_content,
    upgrade_content,
    validate_persisted_state,
    validate_state,
)
from api.game.engine.campaign import RandomOutcome, RandomTable
from api.game.models.api import ErrorCategory
from api.game.models.domain import (
    CombatState,
    Lifecycle,
    SessionIdentity,
    SessionState,
    TemporaryEffect,
    frozen_equipment,
    frozen_items,
)
from api.game.store import GameStore

NOW = datetime(2026, 1, 2, 3, 4, 5, tzinfo=timezone.utc)
IDENTITY = ("libera", "unregistered_nick", "alice")


def configured(*, countdown: int = 6, campaign: CampaignConfig | None = None) -> GameConfigSnapshot:
    return GameConfigSnapshot(
        enabled=True,
        database_path="data/game.db",
        database_pool_size=2,
        campaign=campaign or CampaignConfig(starting_countdown=countdown),
    )


def context(
    config: GameConfigSnapshot,
    content,
    values: tuple[int, ...] = (),
    expected: tuple[tuple[str, str, str], ...] | None = None,
) -> EngineContext:
    return EngineContext(
        config=config,
        content=ContentSnapshot(version=content.version, records=(("campaign", content),)),
        random=ScriptedRandomSource(values, expected=expected),
        now=NOW,
        identity=IDENTITY,
    )


def action(name: str, **arguments: str | int | bool) -> NormalizedAction:
    return NormalizedAction(name=name, arguments=tuple(sorted(arguments.items())))


def apply(
    engine: CampaignEngine,
    state: SessionState | None,
    name: str,
    config: GameConfigSnapshot,
    content,
    *,
    values: tuple[int, ...] = (),
    expected: tuple[tuple[str, str, str], ...] | None = None,
    **arguments: str | int | bool,
):
    return engine.transition(
        state,
        action(name, **arguments),
        context(config, content, values, expected),
    )


def changed_state(result) -> SessionState:
    assert result.state_changed
    assert isinstance(result.next_state, SessionState)
    return result.next_state


def test_configured_initialization_is_exact_and_domain_values_are_immutable() -> None:
    campaign = CampaignConfig(
        starting_health=17,
        starting_max_health=23,
        starting_currency=41,
        starting_inventory=({"item_id": "medkit", "quantity": 3},),
        starting_level=4,
        starting_experience=77,
        starting_day=5,
        starting_countdown=8,
    )
    config = configured(campaign=campaign)
    content = default_campaign_content("init-v1")
    result = apply(CampaignEngine(), None, "start", config, content)
    state = changed_state(result)

    assert state == SessionState(
        identity=SessionIdentity(*IDENTITY),
        state_revision=1,
        lifecycle=Lifecycle.ACTIVE,
        location_id="haven",
        day=5,
        countdown_remaining=8,
        health=17,
        max_health=23,
        currency=41,
        progression_level=4,
        experience=77,
        inventory=(("medkit", 3),),
        selected_content_profile="standard",
        engine_version="1",
        content_version="init-v1",
        state_schema_version=1,
    )
    assert result.result_category == "campaign_started"
    assert {"look", "advance", "travel", "buy", "sell", "use"} == {
        choice.action for choice in result.choices
    }
    with pytest.raises(FrozenInstanceError):
        state.health = 1


def test_scripted_randomness_enforces_bounded_labeled_order_and_table_version() -> None:
    source = ScriptedRandomSource(
        (1,), expected=(("travel.encounter", "travel_encounters", "1"),),
    )
    draw = source.bounded_int(
        "travel.encounter", 2, table_id="travel_encounters", table_version="1",
    )
    assert draw.value == 1
    assert draw.metadata() == {
        "label": "travel.encounter",
        "upper_exclusive": 2,
        "value": 1,
        "table_id": "travel_encounters",
        "table_version": "1",
    }
    assert source.consumed == 1
    with pytest.raises(ValueError, match="equal length"):
        ScriptedRandomSource((0,), expected=())
    with pytest.raises(ValueError, match="require a label"):
        ScriptedRandomSource((0,)).bounded_int("", 1, table_id="table", table_version="1")


def test_complete_campaign_path_gates_clues_rewards_and_final_state() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content()
    state = changed_state(apply(engine, None, "start", config, content))
    initial_clock = (state.day, state.countdown_remaining)

    with pytest.raises(GameServiceError) as locked:
        apply(engine, state, "travel", config, content, destination_id="archive")
    assert locked.value.category == ErrorCategory.INVALID_INPUT

    state = changed_state(apply(
        engine, state, "travel", config, content,
        values=(0,), expected=(("travel.encounter", "travel_encounters", "1"),),
        destination_id="docks",
    ))
    state = changed_state(apply(
        engine, state, "investigate", config, content,
        values=(1,), expected=(("investigate.reward", "investigation_rewards", "1"),),
    ))
    assert "clue_dock_signal" in state.quest_flags
    assert state.currency == config.campaign.starting_currency + 3

    for destination in ("haven", "archive"):
        state = changed_state(apply(
            engine, state, "travel", config, content,
            values=(0,), expected=(("travel.encounter", "travel_encounters", "1"),),
            destination_id=destination,
        ))
    state = changed_state(apply(engine, state, "investigate", config, content))
    assert {"clue_dock_signal", "clue_archive_cipher"}.issubset(state.quest_flags)

    for destination in ("haven", "spire"):
        state = changed_state(apply(
            engine, state, "travel", config, content,
            values=(0,), expected=(("travel.encounter", "travel_encounters", "1"),),
            destination_id=destination,
        ))
    victory = apply(
        engine, state, "finalize", config, content,
        values=(0,), expected=(("final.encounter", "final_encounter", "1"),),
    )
    state = changed_state(victory)

    assert (state.day, state.countdown_remaining) == initial_clock
    assert state.lifecycle == Lifecycle.COMPLETED
    assert state.location_id == "spire"
    assert "final_victory" in state.claimed_rewards
    assert victory.milestones == ("campaign_completed",)
    assert tuple(choice.action for choice in victory.choices) == config.campaign.post_game_choices


def test_recovery_and_only_explicit_advance_change_game_time_until_failure() -> None:
    engine = CampaignEngine()
    config = configured(countdown=2)
    base = default_campaign_content("clock-v1")
    content = replace(base, known_effects=frozenset({"focus"}))
    state = changed_state(apply(engine, None, "start", config, content))

    state = changed_state(apply(
        engine, state, "travel", config, content,
        values=(0,), destination_id="clinic",
    ))
    state = replace(
        state,
        health=5,
        temporary_effects=(TemporaryEffect("focus", state.day + 1),),
    )
    recovered = changed_state(apply(engine, state, "recover", config, content))
    assert recovered.health == 13
    assert (recovered.day, recovered.countdown_remaining) == (1, 2)

    first = changed_state(apply(engine, recovered, "advance", config, content))
    assert (first.day, first.countdown_remaining, first.health) == (2, 1, 15)
    assert first.temporary_effects == ()
    assert "day_two_warning" in first.quest_flags

    failure_result = apply(engine, first, "advance", config, content)
    failed = changed_state(failure_result)
    assert (failed.day, failed.countdown_remaining) == (3, 0)
    assert failed.lifecycle == Lifecycle.FAILED
    assert failure_result.result_category == "campaign_failed"
    assert tuple(choice.action for choice in failure_result.choices) == ("status", "credits", "reset")


def test_identical_inputs_and_scripted_draws_produce_equal_results_and_states() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("deterministic-v1")
    state = changed_state(apply(engine, None, "start", config, content))
    expected = (("travel.encounter", "travel_encounters", "1"),)

    first = apply(
        engine, state, "travel", config, content,
        values=(1,), expected=expected, destination_id="docks",
    )
    second = apply(
        engine, state, "travel", config, content,
        values=(1,), expected=expected, destination_id="docks",
    )
    assert first == second
    assert first.next_state == second.next_state
    assert first.random_metadata == second.random_metadata


def test_state_and_content_upgrade_mappings_cover_all_persisted_identifier_kinds() -> None:
    base = default_campaign_content("standard-v2")
    content = replace(
        base,
        known_abilities=frozenset({"scan"}),
        known_encounters=frozenset({"raider"}),
        known_effects=frozenset({"focus"}),
        equipment_slots=frozenset({"tool"}),
        upgrades=(ContentUpgrade(
            from_version="legacy-v1",
            location_ids=(("old_haven", "haven"),),
            item_ids=(("old_kit", "medkit"),),
            equipment_slot_ids=(("old_tool", "tool"),),
            ability_ids=(("old_scan", "scan"),),
            quest_flag_ids=(("old_clue", "clue_dock_signal"),),
            encounter_ids=(("old_raider", "raider"),),
            effect_ids=(("old_focus", "focus"),),
            reward_ids=(("old_reward", "final_victory"),),
            profile_ids=(("old_standard", "standard"),),
        ),),
    )
    legacy = SessionState(
        identity=SessionIdentity(*IDENTITY),
        state_revision=9,
        lifecycle=Lifecycle.ACTIVE,
        location_id="old_haven",
        day=1,
        countdown_remaining=5,
        health=10,
        max_health=20,
        currency=2,
        progression_level=1,
        experience=0,
        abilities=frozenset({"old_scan"}),
        inventory=frozen_items({"old_kit": 1}),
        equipped=frozen_equipment({"old_tool": "old_kit"}),
        quest_flags=frozenset({"old_clue"}),
        claimed_rewards=frozenset({"old_reward"}),
        combat=CombatState("old_raider", "1", 4),
        temporary_effects=(TemporaryEffect("old_focus", 2),),
        selected_content_profile="old_standard",
        content_version="legacy-v1",
    )

    outcome = upgrade_content(legacy, content)
    assert not outcome.recovery_required
    upgraded = outcome.state
    assert upgraded.state_revision == legacy.state_revision
    assert upgraded.location_id == "haven"
    assert upgraded.inventory == (("medkit", 1),)
    assert upgraded.equipped == (("tool", "medkit"),)
    assert upgraded.abilities == frozenset({"scan"})
    assert upgraded.quest_flags == frozenset({"clue_dock_signal"})
    assert upgraded.claimed_rewards == frozenset({"final_victory"})
    assert upgraded.combat and upgraded.combat.encounter_id == "raider"
    assert upgraded.temporary_effects == (TemporaryEffect("focus", 2),)
    assert upgraded.selected_content_profile == "standard"
    assert upgraded.content_version == "standard-v2"
    validate_state(upgraded, configured().campaign, content)

    registry = StateUpgradeRegistry(2, {1: lambda state: replace(state, state_schema_version=2)})
    schema = registry.upgrade(replace(upgraded, state_schema_version=1))
    assert not schema.recovery_required
    assert schema.state.state_schema_version == 2
    assert schema.state.state_revision == upgraded.state_revision


def test_unmapped_content_enters_non_destructive_recovery_and_invalid_ids_fail_closed() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("standard-v2")
    started = changed_state(apply(engine, None, "start", config, content))
    incompatible = replace(
        started,
        location_id="removed_location",
        inventory=(("removed_item", 1),),
        content_version="legacy-unmapped",
    )
    recovery_result = apply(engine, incompatible, "status", config, content)
    recovery = changed_state(recovery_result)
    assert recovery.lifecycle == Lifecycle.RECOVERY_REQUIRED
    assert recovery.location_id == "removed_location"
    assert recovery.inventory == (("removed_item", 1),)
    assert recovery.content_version == "legacy-unmapped"
    assert recovery.state_revision == incompatible.state_revision + 1
    assert tuple(choice.action for choice in recovery_result.choices) == ("status", "credits")

    invalid_current = replace(started, location_id="unknown", content_version=content.version)
    with pytest.raises(GameServiceError) as invalid:
        apply(engine, invalid_current, "status", config, content)
    assert invalid.value.category == ErrorCategory.ENGINE_INVARIANT_ERROR

    duplicate_serialized = {
        **started.__dict__,
        "identity": started.identity.__dict__,
        "lifecycle": started.lifecycle.value,
        "claimed_rewards": ["final_victory", "final_victory"],
    }
    with pytest.raises(GameServiceError) as duplicate:
        engine.transition(duplicate_serialized, action("status"), context(config, content))
    assert duplicate.value.category == ErrorCategory.ENGINE_INVARIANT_ERROR


def store_request(revision: int, name: str, **arguments: str) -> StoreActionRequest:
    return StoreActionRequest(
        request_id=uuid4(),
        idempotency_key=uuid4(),
        network_id=IDENTITY[0],
        identity_kind=IDENTITY[1],
        identity_value=IDENTITY[2],
        expected_state_revision=revision,
        action=action(name, **arguments),
        configuration_revision=1,
        content_policy_revision=1,
        display_nick="Alice",
        engine_version="1",
        content_version="rollback-v1",
        state_schema_version=1,
    )


def test_invalid_engine_post_state_aborts_the_real_persistence_transaction(tmp_path: Path) -> None:
    config = configured()
    base = default_campaign_content("rollback-v1")
    bad_table = RandomTable(
        "travel_encounters",
        "bad-v1",
        (RandomOutcome("bad_reward", "Invalid reward.", item_id="unknown_item", item_quantity=1),),
    )
    bad_content = replace(
        base,
        random_tables=tuple(
            bad_table if table.table_id == "travel_encounters" else table
            for table in base.random_tables
        ),
    )
    validator = lambda value, revision: validate_persisted_state(
        value, revision, config.campaign, bad_content,
    )
    store = GameStore.open(config, repository_root=tmp_path, invariant_validator=validator)
    engine = CampaignEngine()
    try:
        start_request = store_request(0, "start")
        started = store.execute(
            start_request,
            lambda state: engine.transition(state, start_request.action, context(config, bad_content)),
        )
        assert started.state_revision == 1

        travel_request = store_request(1, "travel", destination_id="docks")
        with pytest.raises(GameServiceError) as rejected:
            store.execute(
                travel_request,
                lambda state: engine.transition(
                    state,
                    travel_request.action,
                    context(
                        config,
                        bad_content,
                        (0,),
                        (("travel.encounter", "travel_encounters", "bad-v1"),),
                    ),
                ),
            )
        assert rejected.value.category == ErrorCategory.ENGINE_INVARIANT_ERROR
        persisted = store.load_state(*IDENTITY)
        assert persisted["state_revision"] == 1
        assert persisted["location_id"] == "haven"
        assert "unknown_item" not in dict(persisted["inventory"])
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            assert connection.execute("SELECT count(*) FROM action_results").fetchone()[0] == 1
            assert connection.execute("SELECT count(*) FROM game_audits").fetchone()[0] == 1
    finally:
        store.close()
