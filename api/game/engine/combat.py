"""Authoritative deterministic combat resolution and draw ordering."""
from __future__ import annotations

from dataclasses import dataclass, replace

from ..config import CampaignConfig
from ..models.domain import Lifecycle, SessionState
from .campaign import CampaignContent, Encounter, MechanicsError
from .economy import use_consumable
from .progression import apply_grant
from .random_source import RandomDraw, RandomSource


@dataclass(frozen=True)
class CombatTransition:
    state: SessionState
    category: str
    facts: tuple[str, ...]
    draws: tuple[RandomDraw, ...] = ()
    milestones: tuple[str, ...] = ()


def resolve_combat(
    state: SessionState,
    action_name: str,
    arguments: dict[str, object],
    config: CampaignConfig,
    content: CampaignContent,
    random: RandomSource,
) -> CombatTransition:
    if state.combat is None:
        raise MechanicsError("There is no active encounter.")
    encounter = content.encounter(state.combat.encounter_id)
    if state.combat.encounter_version != encounter.version:
        raise ValueError("combat encounter version is incompatible")
    if action_name == "attack":
        return _attack(state, arguments.get("target_id"), config, content, encounter, random)
    if action_name == "defend":
        return _enemy_response(
            state,
            config,
            content,
            encounter,
            random,
            initial_facts=("You take a defensive stance.",),
            damage_reduction=encounter.defend_reduction,
            category="combat_defended",
        )
    if action_name == "use":
        used = use_consumable(state, arguments.get("item_id"), content)
        return _enemy_response(
            used.state,
            config,
            content,
            encounter,
            random,
            initial_facts=used.facts,
            category="combat_item_used",
        )
    if action_name == "escape":
        outcome, draw = content.table(encounter.escape_table_id).draw(random, "combat.escape")
        if outcome.value not in {0, 1}:
            raise ValueError("escape outcome must be zero or one")
        if outcome.value == 1:
            return CombatTransition(
                replace(state, combat=None),
                "combat_escaped",
                (outcome.fact,),
                (draw,),
            )
        response = _enemy_response(
            state,
            config,
            content,
            encounter,
            random,
            initial_facts=(outcome.fact,),
            category="combat_escape_failed",
        )
        return replace(response, draws=(draw, *response.draws))
    raise MechanicsError("That action is not available during combat.")


def _attack(
    state: SessionState,
    target_id: object,
    config: CampaignConfig,
    content: CampaignContent,
    encounter: Encounter,
    random: RandomSource,
) -> CombatTransition:
    if target_id != encounter.encounter_id:
        raise MechanicsError("The attack target is not the current encounter.")
    hit, hit_draw = content.table(encounter.player_hit_table_id).draw(random, "combat.player_hit")
    if hit.value not in {0, 1}:
        raise ValueError("player hit outcome must be zero or one")
    facts: list[str] = [hit.fact]
    draws: list[RandomDraw] = [hit_draw]
    current = state
    if hit.value:
        damage_outcome, damage_draw = content.table(encounter.player_damage_table_id).draw(
            random, "combat.player_damage",
        )
        if damage_outcome.value <= 0:
            raise ValueError("player damage outcome must be positive")
        bonus = sum(
            content.item(item_id).combat_damage_bonus
            for item_id in state.equipped_map().values()
        )
        damage = damage_outcome.value + bonus
        enemy_health = max(0, state.combat.enemy_health - damage)
        current = replace(state, combat=replace(state.combat, enemy_health=enemy_health))
        facts.append(f"Dealt {damage} damage; enemy health is {enemy_health}.")
        draws.append(damage_draw)
        if enemy_health == 0:
            reward, reward_draw = content.table(encounter.reward_table_id).draw(
                random, "combat.reward",
            )
            granted = apply_grant(current, reward, config, content)
            lifecycle = (
                Lifecycle.COMPLETED
                if encounter.completes_campaign or reward.completes_campaign
                else current.lifecycle
            )
            victorious = replace(granted.state, combat=None, lifecycle=lifecycle)
            facts.append(reward.fact)
            facts.extend(f"Progressed to level {level}." for level in granted.level_changes)
            draws.append(reward_draw)
            return CombatTransition(
                victorious,
                "campaign_completed" if lifecycle == Lifecycle.COMPLETED else "combat_victory",
                tuple(facts),
                tuple(draws),
                ("campaign_completed",) if lifecycle == Lifecycle.COMPLETED else (),
            )
    response = _enemy_response(
        current,
        config,
        content,
        encounter,
        random,
        initial_facts=tuple(facts),
        category="combat_hit" if hit.value else "combat_miss",
    )
    return replace(response, draws=(*draws, *response.draws))


def _enemy_response(
    state: SessionState,
    config: CampaignConfig,
    content: CampaignContent,
    encounter: Encounter,
    random: RandomSource,
    *,
    initial_facts: tuple[str, ...],
    damage_reduction: int = 0,
    category: str,
) -> CombatTransition:
    hit, hit_draw = content.table(encounter.enemy_hit_table_id).draw(random, "combat.enemy_hit")
    if hit.value not in {0, 1}:
        raise ValueError("enemy hit outcome must be zero or one")
    facts = [*initial_facts, hit.fact]
    draws: list[RandomDraw] = [hit_draw]
    health = state.health
    if hit.value:
        damage_outcome, damage_draw = content.table(encounter.enemy_damage_table_id).draw(
            random, "combat.enemy_damage",
        )
        if damage_outcome.value <= 0:
            raise ValueError("enemy damage outcome must be positive")
        damage = max(0, damage_outcome.value - max(0, damage_reduction))
        health = max(0, state.health - damage)
        facts.append(f"Received {damage} damage; health is {health}/{state.max_health}.")
        draws.append(damage_draw)
    if health == 0:
        recovered_health = min(state.max_health, max(1, encounter.defeat_health))
        defeated = replace(
            state,
            health=recovered_health,
            currency=max(0, state.currency - max(0, encounter.defeat_currency_loss)),
            location_id=encounter.defeat_location_id,
            combat=None,
        )
        facts.append(f"Defeated; recovered at {encounter.defeat_location_id}.")
        return CombatTransition(defeated, "combat_defeat", tuple(facts), tuple(draws))
    assert state.combat is not None
    continued = replace(
        state,
        health=health,
        combat=replace(state.combat, turn=state.combat.turn + 1),
    )
    return CombatTransition(continued, category, tuple(facts), tuple(draws))


__all__ = ["CombatTransition", "resolve_combat"]
