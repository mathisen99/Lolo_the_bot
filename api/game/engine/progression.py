"""Once-only reward and progression grant application."""
from __future__ import annotations

from dataclasses import dataclass, replace

from ..config import CampaignConfig
from ..models.domain import SessionState
from .campaign import CampaignContent, RandomOutcome
from .economy import add_item_bounded


@dataclass(frozen=True)
class GrantResult:
    state: SessionState
    applied_grants: tuple[str, ...] = ()
    level_changes: tuple[int, ...] = ()


def apply_grant(
    state: SessionState,
    outcome: RandomOutcome,
    config: CampaignConfig,
    content: CampaignContent,
) -> GrantResult:
    """Apply an authored grant and every newly crossed threshold at most once."""
    has_effect = any((
        outcome.currency,
        outcome.item_quantity,
        outcome.experience,
        outcome.grants_flag is not None,
    ))
    if has_effect and outcome.grant_id is None:
        raise ValueError("effect-bearing rewards require a stable grant id")
    if outcome.grant_id is not None and outcome.grant_id in state.claimed_rewards:
        return GrantResult(state)

    current = state
    applied: list[str] = []
    if outcome.grant_id is not None:
        applied.append(outcome.grant_id)
    inventory = current.inventory
    if outcome.item_id is not None and outcome.item_quantity:
        inventory, _ = add_item_bounded(
            current, content.item(outcome.item_id), outcome.item_quantity, config, content,
        )
    current = replace(
        current,
        inventory=inventory,
        currency=min(config.maximum_currency, current.currency + max(0, outcome.currency)),
        experience=min(config.maximum_experience, current.experience + max(0, outcome.experience)),
        quest_flags=(
            current.quest_flags | {outcome.grants_flag}
            if outcome.grants_flag is not None else current.quest_flags
        ),
        claimed_rewards=current.claimed_rewards | frozenset(applied),
    )
    progressed = apply_progression(current, config, content)
    return GrantResult(
        progressed.state,
        tuple((*applied, *progressed.applied_grants)),
        progressed.level_changes,
    )


def apply_progression(
    state: SessionState,
    config: CampaignConfig,
    content: CampaignContent,
) -> GrantResult:
    current = state
    applied: list[str] = []
    levels: list[int] = []
    for threshold in sorted(content.progression_thresholds, key=lambda item: item.level):
        if current.experience < threshold.experience_required:
            continue
        if threshold.grant_id in current.claimed_rewards:
            continue
        if threshold.level > config.maximum_level:
            raise ValueError("progression threshold exceeds configured maximum level")
        maximum_health = min(
            config.maximum_health,
            current.max_health + max(0, threshold.max_health_increase),
        )
        health = min(
            maximum_health,
            current.health + max(0, threshold.health_increase),
        )
        inventory = current.inventory
        if threshold.item_id is not None and threshold.item_quantity:
            inventory, _ = add_item_bounded(
                current, content.item(threshold.item_id), threshold.item_quantity, config, content,
            )
        current = replace(
            current,
            progression_level=max(current.progression_level, threshold.level),
            max_health=maximum_health,
            health=health,
            abilities=(
                current.abilities | {threshold.ability_id}
                if threshold.ability_id is not None else current.abilities
            ),
            inventory=inventory,
            currency=min(config.maximum_currency, current.currency + max(0, threshold.currency)),
            claimed_rewards=current.claimed_rewards | {threshold.grant_id},
        )
        applied.append(threshold.grant_id)
        levels.append(threshold.level)
    return GrantResult(current, tuple(applied), tuple(levels))


__all__ = ["GrantResult", "apply_grant", "apply_progression"]
