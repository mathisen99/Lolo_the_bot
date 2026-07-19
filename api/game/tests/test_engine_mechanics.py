from __future__ import annotations

from dataclasses import replace
from datetime import datetime, timezone

import pytest

from api.game.application import ContentSnapshot, EngineContext, GameServiceError, NormalizedAction
from api.game.config import CampaignConfig, GameConfigSnapshot
from api.game.engine import CampaignEngine, ScriptedRandomSource, default_campaign_content
from api.game.models.api import ErrorCategory
from api.game.models.domain import CombatState, Lifecycle, SessionState

NOW = datetime(2026, 2, 3, 4, 5, 6, tzinfo=timezone.utc)
IDENTITY = ("libera", "unregistered_nick", "mechanic")


def configured(campaign: CampaignConfig | None = None) -> GameConfigSnapshot:
    return GameConfigSnapshot(enabled=True, campaign=campaign or CampaignConfig())


def transition(
    engine: CampaignEngine,
    state: SessionState | None,
    name: str,
    config: GameConfigSnapshot,
    content,
    *,
    draws: tuple[int, ...] = (),
    expected: tuple[tuple[str, str, str], ...] | None = None,
    **arguments: str | int,
):
    return engine.transition(
        state,
        NormalizedAction(name, tuple(sorted(arguments.items()))),
        EngineContext(
            config=config,
            content=ContentSnapshot(content.version, records=(("campaign", content),)),
            random=ScriptedRandomSource(draws, expected=expected),
            now=NOW,
            identity=IDENTITY,
        ),
    )


def changed(result) -> SessionState:
    assert result.state_changed
    assert isinstance(result.next_state, SessionState)
    return result.next_state


def start_ambush(engine: CampaignEngine, config: GameConfigSnapshot, content) -> SessionState:
    state = changed(transition(engine, None, "start", config, content))
    return changed(transition(
        engine,
        state,
        "travel",
        config,
        content,
        draws=(2,),
        expected=(("travel.encounter", "travel_encounters", "1"),),
        destination_id="docks",
    ))


def test_combat_victory_draw_order_progression_boundary_and_once_only_grants() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("mechanics-v1")
    state = start_ambush(engine, config, content)
    original_clock = (state.day, state.countdown_remaining)

    victory = transition(
        engine,
        state,
        "attack",
        config,
        content,
        draws=(1, 2, 0),
        expected=(
            ("combat.player_hit", "combat_player_hit", "1"),
            ("combat.player_damage", "combat_player_damage", "1"),
            ("combat.reward", "dock_raider_rewards", "1"),
        ),
        target_id="dock_raider",
    )
    won = changed(victory)
    assert victory.result_category == "combat_victory"
    assert won.combat is None
    assert won.currency == config.campaign.starting_currency + 4
    assert won.experience == 10
    assert won.progression_level == 2
    assert (won.health, won.max_health) == (25, 25)
    assert won.abilities == frozenset({"resolve"})
    assert {"encounter:dock_raider:v1:victory", "progression:level:2"} <= won.claimed_rewards
    assert "dock_raider_defeated" in won.quest_flags
    assert (won.day, won.countdown_remaining) == original_clock

    # Re-entering the same authored encounter cannot duplicate its reward or threshold.
    repeated = replace(
        won,
        combat=CombatState("dock_raider", "1", 1),
    )
    replay = changed(transition(
        engine,
        repeated,
        "attack",
        config,
        content,
        draws=(1, 0, 0),
        target_id="dock_raider",
    ))
    assert (replay.currency, replay.experience, replay.max_health) == (
        won.currency,
        won.experience,
        won.max_health,
    )
    assert replay.claimed_rewards == won.claimed_rewards


def test_combat_defeat_and_escape_are_bounded_and_deterministic() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("combat-outcomes-v1")
    ambush = start_ambush(engine, config, content)

    defeated = changed(transition(
        engine,
        replace(ambush, health=1),
        "attack",
        config,
        content,
        draws=(0, 1, 2),
        expected=(
            ("combat.player_hit", "combat_player_hit", "1"),
            ("combat.enemy_hit", "combat_enemy_hit", "1"),
            ("combat.enemy_damage", "combat_enemy_damage", "1"),
        ),
        target_id="dock_raider",
    ))
    assert defeated.combat is None
    assert defeated.health == 1
    assert defeated.currency == 3
    assert defeated.location_id == "clinic"
    assert defeated.lifecycle == Lifecycle.ACTIVE

    escaped_result = transition(
        engine,
        ambush,
        "escape",
        config,
        content,
        draws=(1,),
        expected=(("combat.escape", "combat_escape", "1"),),
    )
    escaped = changed(escaped_result)
    assert escaped_result.result_category == "combat_escaped"
    assert escaped.combat is None
    assert (escaped.health, escaped.currency) == (ambush.health, ambush.currency)


def test_combat_consumable_is_spent_before_the_enemy_response() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("combat-resource-v1")
    ambush = replace(start_ambush(engine, config, content), health=10)

    result = transition(
        engine,
        ambush,
        "use",
        config,
        content,
        draws=(1, 0),
        expected=(
            ("combat.enemy_hit", "combat_enemy_hit", "1"),
            ("combat.enemy_damage", "combat_enemy_damage", "1"),
        ),
        item_id="medkit",
    )
    state = changed(result)
    assert state.inventory_map().get("medkit", 0) == 0
    assert state.health == 17  # Recover 8 first, then receive 1 damage.
    assert state.combat is not None
    assert state.combat.turn == 2
    assert state.combat.consumables_used == ("medkit",)
    assert result.facts[0].startswith("Used medkit")
    assert (state.day, state.countdown_remaining) == (ambush.day, ambush.countdown_remaining)


def test_atomic_buy_equip_sell_use_and_failure_preserves_input_state() -> None:
    engine = CampaignEngine()
    campaign = CampaignConfig(starting_currency=20)
    config = configured(campaign)
    content = default_campaign_content("economy-v1")
    initial = changed(transition(engine, None, "start", config, content))

    bought = changed(transition(
        engine, initial, "buy", config, content, item_id="pulse_blade", quantity=1,
    ))
    assert bought.currency == 12
    assert bought.inventory_map()["pulse_blade"] == 1
    equipped = changed(transition(
        engine, bought, "equip", config, content, item_id="pulse_blade",
    ))
    assert equipped.equipped_map() == {"weapon": "pulse_blade"}

    with pytest.raises(GameServiceError) as rejected:
        transition(
            engine, equipped, "sell", config, content, item_id="pulse_blade", quantity=1,
        )
    assert rejected.value.category == ErrorCategory.INVALID_INPUT
    assert equipped.currency == 12
    assert equipped.inventory_map()["pulse_blade"] == 1
    assert equipped.equipped_map() == {"weapon": "pulse_blade"}

    used = changed(transition(
        engine,
        replace(equipped, health=10),
        "use",
        config,
        content,
        item_id="medkit",
    ))
    assert used.health == 18
    assert "medkit" not in used.inventory_map()


def test_capacity_failure_and_explicit_day_advance_interaction_are_atomic() -> None:
    engine = CampaignEngine()
    config = configured(CampaignConfig(starting_currency=20, inventory_capacity=1))
    content = default_campaign_content("capacity-clock-v1")
    initial = changed(transition(engine, None, "start", config, content))

    with pytest.raises(GameServiceError) as full:
        transition(engine, initial, "buy", config, content, item_id="medkit", quantity=1)
    assert full.value.category == ErrorCategory.INVALID_INPUT
    assert (initial.currency, initial.inventory) == (20, (("medkit", 1),))

    ambush = changed(transition(
        engine,
        initial,
        "travel",
        config,
        content,
        draws=(2,),
        destination_id="docks",
    ))
    escaped = changed(transition(engine, ambush, "escape", config, content, draws=(1,)))
    advanced = changed(transition(engine, escaped, "advance", config, content))
    assert (ambush.day, ambush.countdown_remaining) == (1, 6)
    assert (escaped.day, escaped.countdown_remaining) == (1, 6)
    assert (advanced.day, advanced.countdown_remaining) == (2, 5)


def test_final_completion_clears_combat_and_blocks_further_mechanics() -> None:
    engine = CampaignEngine()
    config = configured()
    content = default_campaign_content("final-effects-v1")
    started = changed(transition(engine, None, "start", config, content))
    ready = replace(
        started,
        location_id="spire",
        quest_flags=frozenset({"clue_dock_signal", "clue_archive_cipher"}),
    )
    final = transition(
        engine,
        ready,
        "finalize",
        config,
        content,
        draws=(0,),
        expected=(("final.encounter", "final_encounter", "1"),),
    )
    completed = changed(final)
    assert completed.lifecycle == Lifecycle.COMPLETED
    assert completed.combat is None
    assert final.milestones == ("campaign_completed",)
    assert tuple(choice.action for choice in final.choices) == config.campaign.post_game_choices
    assert (completed.day, completed.countdown_remaining) == (ready.day, ready.countdown_remaining)

    with pytest.raises(GameServiceError) as ended:
        transition(
            engine, completed, "buy", config, content, item_id="medkit", quantity=1,
        )
    assert ended.value.category == ErrorCategory.INVALID_INPUT
