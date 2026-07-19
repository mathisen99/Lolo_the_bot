"""Pure deterministic campaign state machine."""
from __future__ import annotations

import base64
import hashlib
from dataclasses import replace
from datetime import timedelta
from typing import Iterable, Mapping

from ..application import (
    AuthoritativeChoice,
    AuthoritativeResult,
    EngineContext,
    GameServiceError,
    NormalizedAction,
)
from ..models.api import ErrorCategory
from ..renderer.credits import render_credits
from ..models.domain import (
    CombatState,
    Lifecycle,
    SessionIdentity,
    SessionState,
    frozen_items,
    state_from_mapping,
)
from .campaign import (
    CampaignContent,
    MechanicsError,
    RandomOutcome,
    default_campaign_content,
    gate_satisfied,
)
from .combat import resolve_combat
from .economy import buy, equip, sell, use_consumable
from .invariants import (
    InvariantViolation,
    validate_serialized_shape,
    validate_state,
    validate_transition,
)
from .random_source import RandomDraw, RandomSource
from .progression import apply_grant
from .upgrades import StateUpgradeRegistry, upgrade_state


class CampaignEngine:
    version = "1"

    def __init__(self, *, schema_upgrades: StateUpgradeRegistry | None = None) -> None:
        self._schema_upgrades = schema_upgrades or StateUpgradeRegistry()

    def transition(
        self,
        state: object | None,
        action: NormalizedAction,
        context: EngineContext,
    ) -> AuthoritativeResult:
        content = self._content(context)
        random = context.random
        if random is None or not hasattr(random, "bounded_int"):
            raise GameServiceError(
                ErrorCategory.GAME_UNAVAILABLE,
                "Campaign randomness is unavailable.",
                retryable=True,
            )
        try:
            before = self._decode_state(state)
            if before is None:
                if action.name == "start":
                    after = self._initial_state(context, content)
                    return self._changed(None, after, action, context, content, "campaign_started", (
                        f"Campaign started at {content.location(after.location_id).display_name}. Health: {after.health}/{after.max_health}.",
                        "Content notice: Standard fictionalized content is active; use content status for policy details or quit to opt out.",
                    ))
                if action.name in {"help", "credits", "privacy", "content", "resume", "status", "inventory"}:
                    return self._without_session(action, context)
                raise self._invalid("Start a campaign before using that action.")

            # Validate intrinsic ranges and relationships before any pure upgrade.
            # Content/schema compatibility is checked after mapping so old IDs are
            # not rejected before an explicit upgrade has a chance to translate them.
            validate_state(
                before,
                context.config.campaign,
                content,
                expected_schema_version=self._schema_upgrades.current_version,
                allow_incompatible_content=True,
            )
            if context.identity is not None:
                expected_identity = SessionIdentity(*context.identity)
                if before.identity != expected_identity:
                    raise InvariantViolation("session identity does not match the transaction identity")

            upgraded = upgrade_state(before, content, self._schema_upgrades)
            if upgraded.recovery_required:
                recovery = replace(upgraded.state, state_revision=before.state_revision + 1)
                return self._changed(
                    before,
                    recovery,
                    action,
                    context,
                    content,
                    "recovery_required",
                    ("This campaign needs an operator content mapping before play can continue.",),
                )
            current = upgraded.state
            validate_state(
                current,
                context.config.campaign,
                content,
                expected_schema_version=self._schema_upgrades.current_version,
            )
            if current.lifecycle == Lifecycle.RECOVERY_REQUIRED:
                raise GameServiceError(
                    ErrorCategory.RECOVERY_REQUIRED,
                    "This campaign requires operator recovery before it can change.",
                    state_revision=current.state_revision,
                )
            if action.name == "quit":
                return self._result(
                    result_category="campaign_quit",
                    state=current,
                    state_changed=False,
                    facts=("Interaction closed. Your campaign progress is saved.",),
                    choices=(),
                    context=context,
                )
            if action.name == "reset":
                token = action.argument_dict().get("token")
                if not isinstance(token, str) or not token:
                    raise self._invalid("Request a reset confirmation token before resetting.", current)
                after = replace(
                    self._initial_state(context, content),
                    state_revision=current.state_revision + 1,
                )
                return self._changed(
                    current,
                    after,
                    action,
                    context,
                    content,
                    "campaign_reset",
                    (f"Campaign reset at {after.location_id}. Health: {after.health}/{after.max_health}.",),
                )
            if action.name in {"status", "look", "inventory", "help", "credits", "privacy", "content", "page", "resume", "start"}:
                return self._read_only(current, action, context, content)
            if current.lifecycle in {Lifecycle.COMPLETED, Lifecycle.FAILED}:
                raise self._invalid("The campaign has ended; use status, credits, or the reset flow.", current)
            if current.combat is not None:
                if action.name not in {"attack", "defend", "use", "escape"}:
                    raise self._invalid("Resolve the current encounter before taking a campaign action.", current)
                return self._combat(current, action, context, content, random)

            if action.name == "travel":
                return self._travel(current, action, context, content, random)
            if action.name == "investigate":
                return self._investigate(current, action, context, content, random)
            if action.name in {"buy", "sell", "equip", "use"}:
                return self._economy(current, action, context, content)
            if action.name == "recover":
                return self._recover(current, action, context, content)
            if action.name == "advance":
                return self._advance(current, action, context, content)
            if action.name == "finalize":
                return self._finalize(current, action, context, content, random)
            raise self._invalid("That action is not available in the current campaign state.", current)
        except GameServiceError:
            raise
        except MechanicsError as exc:
            raise self._invalid(str(exc), locals().get("before")) from exc
        except InvariantViolation as exc:
            raise GameServiceError(
                ErrorCategory.ENGINE_INVARIANT_ERROR,
                "The game rejected an invalid state transition.",
                state_revision=getattr(locals().get("before"), "state_revision", 0),
            ) from exc
        except (KeyError, ValueError) as exc:
            raise GameServiceError(
                ErrorCategory.ENGINE_INVARIANT_ERROR,
                "The game rejected invalid campaign content or state.",
                state_revision=getattr(locals().get("before"), "state_revision", 0),
            ) from exc

    @staticmethod
    def _content(context: EngineContext) -> CampaignContent:
        records = dict(context.content.records)
        content = records.get("campaign")
        if content is None:
            return default_campaign_content(context.content.version)
        if not isinstance(content, CampaignContent):
            raise ValueError("campaign content record has an invalid type")
        if content.version != context.content.version:
            raise ValueError("content snapshot version mismatch")
        return content

    @staticmethod
    def _decode_state(state: object | None) -> SessionState | None:
        if state is None:
            return None
        if isinstance(state, SessionState):
            return state
        if isinstance(state, Mapping):
            validate_serialized_shape(state)
            return state_from_mapping(state)
        raise InvariantViolation("session state has an invalid representation")

    def _initial_state(self, context: EngineContext, content: CampaignContent) -> SessionState:
        if context.identity is None:
            raise InvariantViolation("new session identity is missing")
        campaign = context.config.campaign
        state = SessionState(
            identity=SessionIdentity(*context.identity),
            state_revision=1,
            lifecycle=Lifecycle.ACTIVE,
            location_id=campaign.starting_location,
            day=campaign.starting_day,
            countdown_remaining=campaign.starting_countdown,
            health=campaign.starting_health,
            max_health=campaign.starting_max_health,
            currency=campaign.starting_currency,
            progression_level=campaign.starting_level,
            experience=campaign.starting_experience,
            inventory=frozen_items(campaign.inventory_map()),
            selected_content_profile=context.config.standard_content_profile,
            engine_version=self.version,
            content_version=content.version,
            state_schema_version=self._schema_upgrades.current_version,
        )
        validate_transition(
            None,
            state,
            campaign,
            content,
            action_name="start",
            expected_schema_version=self._schema_upgrades.current_version,
        )
        return state

    def _travel(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        random: RandomSource,
    ) -> AuthoritativeResult:
        destination = action.argument_dict().get("destination_id")
        if not isinstance(destination, str):
            raise self._invalid("Travel requires a destination.", state)
        edge = next((item for item in content.location(state.location_id).edges if item.destination_id == destination), None)
        if edge is None:
            raise self._invalid("That destination is not connected to the current location.", state)
        if not gate_satisfied(state, edge.gate):
            raise self._invalid("That route is still locked by campaign prerequisites.", state)
        outcome, draw = content.table("travel_encounters").draw(random, "travel.encounter")
        granted = apply_grant(state, outcome, context.config.campaign, content)
        combat = granted.state.combat
        if outcome.encounter_id is not None:
            encounter = content.encounter(outcome.encounter_id)
            combat = CombatState(
                encounter_id=encounter.encounter_id,
                encounter_version=encounter.version,
                enemy_health=encounter.enemy_max_health,
            )
        after = replace(
            granted.state,
            state_revision=state.state_revision + 1,
            location_id=destination,
            combat=combat,
            engine_version=self.version,
            content_version=content.version,
        )
        return self._changed(
            state, after, action, context, content, "travel", (f"Travelled to {content.location(destination).display_name}.", outcome.fact), (draw,),
        )

    def _investigate(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        random: RandomSource,
    ) -> AuthoritativeResult:
        location = content.location(state.location_id)
        if location.investigation_id is None:
            raise self._invalid("There is no investigation available here.", state)
        investigation = content.investigation(location.investigation_id)
        if investigation.grants_flag in state.quest_flags:
            return self._read_only(state, action, context, content, facts=("This location has already been fully investigated.",))
        facts = [investigation.fact or "You discover a useful campaign clue."]
        granted = apply_grant(
            state,
            RandomOutcome(
                investigation.investigation_id,
                "",
                experience=investigation.experience,
                grants_flag=investigation.grants_flag,
                grant_id=investigation.grant_id,
            ),
            context.config.campaign,
            content,
        )
        draws: tuple[RandomDraw, ...] = ()
        if investigation.reward_table_id:
            outcome, draw = content.table(investigation.reward_table_id).draw(random, "investigate.reward")
            granted = apply_grant(granted.state, outcome, context.config.campaign, content)
            facts.append(outcome.fact)
            draws = (draw,)
        facts.extend(f"Progressed to level {level}." for level in granted.level_changes)
        after = replace(
            granted.state,
            state_revision=state.state_revision + 1,
            engine_version=self.version,
            content_version=content.version,
        )
        return self._changed(state, after, action, context, content, "clue_discovered", tuple(facts), draws)

    def _economy(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
    ) -> AuthoritativeResult:
        arguments = action.argument_dict()
        if action.name == "buy":
            transition = buy(
                state, arguments.get("item_id"), arguments.get("quantity"),
                context.config.campaign, content,
            )
        elif action.name == "sell":
            transition = sell(state, arguments.get("item_id"), arguments.get("quantity"), content)
        elif action.name == "equip":
            transition = equip(state, arguments.get("item_id"), content)
        else:
            transition = use_consumable(state, arguments.get("item_id"), content)
        after = replace(
            transition.state,
            state_revision=state.state_revision + 1,
            engine_version=self.version,
            content_version=content.version,
        )
        return self._changed(
            state, after, action, context, content, transition.category, transition.facts,
        )

    def _combat(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        random: RandomSource,
    ) -> AuthoritativeResult:
        transition = resolve_combat(
            state,
            action.name,
            action.argument_dict(),
            context.config.campaign,
            content,
            random,
        )
        after = replace(
            transition.state,
            state_revision=state.state_revision + 1,
            engine_version=self.version,
            content_version=content.version,
        )
        return self._changed(
            state,
            after,
            action,
            context,
            content,
            transition.category,
            transition.facts,
            transition.draws,
            transition.milestones,
        )

    def _recover(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
    ) -> AuthoritativeResult:
        if not content.location(state.location_id).recovery_allowed:
            raise self._invalid("Recovery is available only at a recovery location.", state)
        health = min(state.max_health, state.health + context.config.campaign.recovery_action_amount)
        if health == state.health:
            return self._read_only(state, action, context, content, facts=("Health is already full.",))
        after = replace(state, state_revision=state.state_revision + 1, health=health, engine_version=self.version)
        return self._changed(state, after, action, context, content, "recovered", (f"Recovered to {health}/{state.max_health} health.",))

    def _advance(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
    ) -> AuthoritativeResult:
        day = state.day + 1
        countdown = max(0, state.countdown_remaining - 1)
        flags = set(state.quest_flags)
        facts = [f"Day advanced to {day}; countdown: {countdown}."]
        for event in content.day_events:
            if event.day == day and event.grants_flag not in flags:
                flags.add(event.grants_flag)
                facts.append(event.fact or f"Campaign event: {event.event_id}.")
        lifecycle = Lifecycle.FAILED if countdown == 0 else state.lifecycle
        if lifecycle == Lifecycle.FAILED:
            facts.append("The countdown expired; the campaign has failed.")
        after = replace(
            state,
            state_revision=state.state_revision + 1,
            lifecycle=lifecycle,
            day=day,
            countdown_remaining=countdown,
            health=min(state.max_health, state.health + context.config.campaign.recovery_per_advance),
            quest_flags=frozenset(flags),
            temporary_effects=tuple(effect for effect in state.temporary_effects if effect.expires_on_day > day),
            combat=None if lifecycle == Lifecycle.FAILED else state.combat,
            engine_version=self.version,
        )
        return self._changed(state, after, action, context, content, "campaign_failed" if lifecycle == Lifecycle.FAILED else "day_advanced", tuple(facts))

    def _finalize(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        random: RandomSource,
    ) -> AuthoritativeResult:
        if state.location_id != content.final_location_id or not gate_satisfied(state, content.final_gate):
            raise self._invalid("The final encounter is still locked.", state)
        outcome, draw = content.table(content.final_table_id).draw(random, "final.encounter")
        granted = apply_grant(state, outcome, context.config.campaign, content)
        lifecycle = Lifecycle.COMPLETED if outcome.completes_campaign else Lifecycle.ACTIVE
        after = replace(
            granted.state,
            state_revision=state.state_revision + 1,
            lifecycle=lifecycle,
            combat=None,
            engine_version=self.version,
        )
        milestones = ("campaign_completed",) if lifecycle == Lifecycle.COMPLETED else ()
        return self._changed(
            state,
            after,
            action,
            context,
            content,
            "campaign_completed" if lifecycle == Lifecycle.COMPLETED else "final_setback",
            (outcome.fact,),
            (draw,),
            milestones,
        )

    def _without_session(
        self,
        action: NormalizedAction,
        context: EngineContext,
    ) -> AuthoritativeResult:
        fallback = bool(action.argument_dict().get("fallback"))
        if action.name == "credits":
            facts = (self._credits_text(),)
            category = "credits"
        elif action.name == "privacy":
            facts = (self._privacy_text(),)
            category = "privacy"
        elif action.name == "content":
            facts = (self._content_text(),)
            category = "content"
        elif action.name == "inventory":
            facts = ("No campaign inventory exists yet; start a campaign first.",)
            category = "inventory"
        elif action.name in {"resume", "status"}:
            facts = ("No campaign exists yet; choose start.",)
            category = "status"
        else:
            facts = self._help_text(fallback)
            category = "unknown_action" if fallback else "help"
        choice = AuthoritativeChoice(input="start", kind="action", action="start")
        return AuthoritativeResult(
            result_category=category,
            state_revision=0,
            state_changed=False,
            facts=facts,
            choices=(choice,),
            menu_context_id=f"m-r0-{category}",
            menu_expires_at=context.now + timedelta(seconds=context.config.menu_context_ttl_seconds),
        )

    @staticmethod
    def _help_text(fallback: bool = False) -> tuple[str, ...]:
        lead = "That action is not recognized. " if fallback else ""
        return (
            lead + "Stable commands: start, status, inventory, help, credits, reset, and quit.",
            "Use the avenger command followed by a displayed action; single-word choices also work as PM replies. Menus support next, prev, and page N.",
        )

    @staticmethod
    def _credits_text() -> str:
        return render_credits()

    @staticmethod
    def _privacy_text() -> str:
        return (
            "Privacy: the game stores your network-scoped identity key, campaign state, content preferences, "
            "normalized action metadata, revisions, and operational audit fields; it does not store arbitrary PM "
            "conversation or unregistered hostmasks. Contact an operator for authenticated deletion."
        )

    @staticmethod
    def _content_text() -> str:
        return (
            "Content profile: Standard fictionalized content; explicit sexual content, graphic gore, targeted "
            "abuse, and instructional non-consensual drugging are excluded. Quit to stop play."
        )

    def _read_only(
        self,
        state: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        *,
        facts: tuple[str, ...] | None = None,
    ) -> AuthoritativeResult:
        choices = self._choices(state, content, context)
        page = 1
        fallback = bool(action.argument_dict().get("fallback"))
        if action.name == "page":
            requested = action.argument_dict().get("page")
            total_pages = max(1, (len(choices) + context.config.page_size - 1) // context.config.page_size)
            if not isinstance(requested, int) or requested < 1 or requested > total_pages:
                raise self._invalid("That menu page is unavailable; use the current menu.", state)
            page = requested
            facts = (f"Menu page {page} of {total_pages}.",)
        elif facts is None:
            if action.name == "inventory":
                summary = ", ".join(f"{item} x{quantity}" for item, quantity in state.inventory) or "empty"
                equipped = ", ".join(f"{slot}: {item}" for slot, item in state.equipped) or "none"
                facts = (f"Inventory: {summary}. Equipped: {equipped}.",)
            elif action.name == "credits":
                facts = (self._credits_text(),)
            elif action.name == "privacy":
                facts = (self._privacy_text(),)
            elif action.name == "content":
                facts = (self._content_text(),)
            elif action.name == "help":
                facts = self._help_text(fallback)
            elif action.name == "look":
                location = content.location(state.location_id)
                facts = (
                    f"Location: {location.display_name}. {location.description} "
                    f"Day {state.day}; countdown {state.countdown_remaining}.",
                )
            else:
                facts = (
                    f"Status: {state.lifecycle.value} at {state.location_id}; day {state.day}; countdown "
                    f"{state.countdown_remaining}; health {state.health}/{state.max_health}; currency "
                    f"{state.currency}; level {state.progression_level}; experience {state.experience}.",
                )
        category = "campaign_resumed" if action.name == "start" else (
            "status" if action.name in {"status", "resume"} else
            "unknown_action" if action.name == "help" and fallback else action.name
        )
        return self._result(
            result_category=category,
            state=state,
            state_changed=False,
            facts=facts,
            choices=choices,
            context=context,
            menu_page=page,
        )

    def _changed(
        self,
        before: SessionState | None,
        after: SessionState,
        action: NormalizedAction,
        context: EngineContext,
        content: CampaignContent,
        category: str,
        facts: tuple[str, ...],
        draws: tuple[RandomDraw, ...] = (),
        milestones: tuple[str, ...] = (),
    ) -> AuthoritativeResult:
        validate_transition(
            before,
            after,
            context.config.campaign,
            content,
            action_name=action.name,
            expected_schema_version=self._schema_upgrades.current_version,
        )
        return self._result(
            result_category=category,
            state=after,
            state_changed=True,
            facts=facts,
            choices=self._choices(after, content, context),
            context=context,
            draws=draws,
            milestones=milestones,
        )

    def _result(
        self,
        *,
        result_category: str,
        state: SessionState,
        state_changed: bool,
        facts: tuple[str, ...],
        choices: tuple[AuthoritativeChoice, ...],
        context: EngineContext,
        draws: tuple[RandomDraw, ...] = (),
        milestones: tuple[str, ...] = (),
        menu_page: int = 1,
    ) -> AuthoritativeResult:
        context_id = f"m-r{state.state_revision}-p{menu_page}-{result_category[:20].replace('_', '-')}" if choices else None
        expires = context.now + timedelta(seconds=context.config.menu_context_ttl_seconds) if choices else None
        return AuthoritativeResult(
            result_category=result_category,
            state_revision=state.state_revision,
            state_changed=state_changed,
            facts=facts,
            choices=choices,
            menu_context_id=context_id,
            menu_page=menu_page,
            menu_expires_at=expires,
            milestones=milestones,
            next_state=state if state_changed else None,
            random_metadata=(("draws", tuple(draw.metadata() for draw in draws)),) if draws else (),
        )

    def _choices(
        self,
        state: SessionState,
        content: CampaignContent,
        context: EngineContext,
    ) -> tuple[AuthoritativeChoice, ...]:
        if state.lifecycle in {Lifecycle.COMPLETED, Lifecycle.FAILED}:
            return tuple(
                self._choice(state, name)
                for name in context.config.campaign.post_game_choices
            )
        if state.lifecycle == Lifecycle.RECOVERY_REQUIRED:
            return tuple(self._choice(state, name) for name in ("status", "credits"))
        if state.combat is not None:
            choices = [
                self._choice(state, "attack", target_id=state.combat.encounter_id),
                self._choice(state, "defend"),
                self._choice(state, "escape"),
            ]
            for item_id, quantity in state.inventory:
                item = content.item(item_id)
                if quantity and item.combat_usable and state.health < state.max_health:
                    choices.append(self._choice(state, "use", item_id=item_id))
            return tuple(choices)
        choices: list[AuthoritativeChoice] = [self._choice(state, "look"), self._choice(state, "advance")]
        location = content.location(state.location_id)
        for edge in location.edges:
            if gate_satisfied(state, edge.gate):
                choices.append(self._choice(state, "travel", destination_id=edge.destination_id))
        if location.investigation_id:
            investigation = content.investigation(location.investigation_id)
            if investigation.grants_flag not in state.quest_flags:
                choices.append(self._choice(state, "investigate"))
        if location.recovery_allowed and state.health < state.max_health:
            choices.append(self._choice(state, "recover"))
        if state.health < state.max_health:
            for item_id, quantity in state.inventory:
                if quantity and content.item(item_id).recovery_amount > 0:
                    choices.append(self._choice(state, "use", item_id=item_id))
        if location.commerce_allowed:
            for item in content.items:
                choices.append(self._choice(state, "buy", item_id=item.item_id, quantity=1))
            for item_id, quantity in state.inventory:
                item = content.item(item_id)
                if quantity and item.sell_price >= 0:
                    choices.append(self._choice(state, "sell", item_id=item_id, quantity=1))
                if item.equipment_slot is not None and state.equipped_map().get(item.equipment_slot) != item_id:
                    choices.append(self._choice(state, "equip", item_id=item_id))
        if state.location_id == content.final_location_id and gate_satisfied(state, content.final_gate):
            choices.append(self._choice(state, "finalize"))
        return tuple(choices)

    @staticmethod
    def _choice(
        state: SessionState,
        action: str,
        **arguments: str | int | bool,
    ) -> AuthoritativeChoice:
        if arguments:
            seed = f"{state.state_revision}:{state.location_id}:{action}:{sorted(arguments.items())}"
            encoded = base64.b32encode(hashlib.sha256(seed.encode()).digest()[:5]).decode().lower().rstrip("=")
            token = f"c-{encoded}"
            return AuthoritativeChoice(
                input=token,
                kind="choice",
                action=action,
                arguments=tuple(sorted(arguments.items())),
                choice_token=token,
            )
        return AuthoritativeChoice(input=action, kind="action", action=action)

    @staticmethod
    def _invalid(message: str, state: SessionState | None = None) -> GameServiceError:
        return GameServiceError(
            ErrorCategory.INVALID_INPUT,
            message,
            state_revision=state.state_revision if state else 0,
        )


__all__ = ["CampaignEngine"]
