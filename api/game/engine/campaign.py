"""Declarative campaign domain values and prerequisite evaluation."""
from __future__ import annotations

from dataclasses import dataclass, replace

from ..models.domain import Lifecycle, SessionState
from .random_source import RandomDraw, RandomSource


class MechanicsError(ValueError):
    """A validly shaped action that is unavailable for the current state."""


@dataclass(frozen=True)
class Gate:
    required_flags: frozenset[str] = frozenset()
    required_items: tuple[tuple[str, int], ...] = ()
    required_location: str | None = None
    minimum_level: int = 1
    lifecycle: Lifecycle = Lifecycle.ACTIVE
    profile: str | None = None


@dataclass(frozen=True)
class TravelEdge:
    destination_id: str
    gate: Gate = Gate()


@dataclass(frozen=True)
class Location:
    location_id: str
    edges: tuple[TravelEdge, ...]
    recovery_allowed: bool = False
    commerce_allowed: bool = False
    investigation_id: str | None = None
    display_name: str = ""
    description: str = ""


@dataclass(frozen=True)
class Investigation:
    investigation_id: str
    location_id: str
    grants_flag: str
    grant_id: str
    experience: int = 0
    reward_table_id: str | None = None
    fact: str = ""


@dataclass(frozen=True)
class ItemDefinition:
    item_id: str
    price: int
    sell_price: int
    capacity_cost: int = 1
    recovery_amount: int = 0
    equipment_slot: str | None = None
    combat_damage_bonus: int = 0
    combat_usable: bool = False
    display_name: str = ""


@dataclass(frozen=True)
class ProgressionThreshold:
    level: int
    experience_required: int
    grant_id: str
    max_health_increase: int = 0
    health_increase: int = 0
    ability_id: str | None = None
    currency: int = 0
    item_id: str | None = None
    item_quantity: int = 0


@dataclass(frozen=True)
class Encounter:
    encounter_id: str
    version: str
    enemy_max_health: int
    player_hit_table_id: str
    player_damage_table_id: str
    enemy_hit_table_id: str
    enemy_damage_table_id: str
    escape_table_id: str
    reward_table_id: str
    defeat_location_id: str
    defeat_health: int = 1
    defeat_currency_loss: int = 0
    defend_reduction: int = 1
    completes_campaign: bool = False
    display_name: str = ""


@dataclass(frozen=True)
class RandomOutcome:
    outcome_id: str
    fact: str
    value: int = 0
    currency: int = 0
    item_id: str | None = None
    item_quantity: int = 0
    experience: int = 0
    grants_flag: str | None = None
    encounter_id: str | None = None
    grant_id: str | None = None
    completes_campaign: bool = False


@dataclass(frozen=True)
class RandomTable:
    table_id: str
    version: str
    outcomes: tuple[RandomOutcome, ...]

    def draw(self, source: RandomSource, label: str) -> tuple[RandomOutcome, RandomDraw]:
        if not self.outcomes:
            raise ValueError(f"random table {self.table_id} is empty")
        draw = source.bounded_int(
            label,
            len(self.outcomes),
            table_id=self.table_id,
            table_version=self.version,
        )
        return self.outcomes[draw.value], draw


@dataclass(frozen=True)
class DayEvent:
    event_id: str
    day: int
    grants_flag: str
    fact: str = ""


@dataclass(frozen=True)
class QuestImplication:
    consequence_flag: str
    required_flags: frozenset[str]


@dataclass(frozen=True)
class ContentUpgrade:
    from_version: str
    location_ids: tuple[tuple[str, str], ...] = ()
    item_ids: tuple[tuple[str, str], ...] = ()
    equipment_slot_ids: tuple[tuple[str, str], ...] = ()
    ability_ids: tuple[tuple[str, str], ...] = ()
    quest_flag_ids: tuple[tuple[str, str], ...] = ()
    encounter_ids: tuple[tuple[str, str], ...] = ()
    effect_ids: tuple[tuple[str, str], ...] = ()
    reward_ids: tuple[tuple[str, str], ...] = ()
    profile_ids: tuple[tuple[str, str], ...] = ()


@dataclass(frozen=True)
class CampaignContent:
    version: str
    locations: tuple[Location, ...]
    investigations: tuple[Investigation, ...]
    random_tables: tuple[RandomTable, ...]
    final_location_id: str
    final_gate: Gate
    final_table_id: str
    known_items: frozenset[str]
    known_abilities: frozenset[str]
    known_encounters: frozenset[str]
    known_effects: frozenset[str]
    available_profiles: frozenset[str]
    equipment_slots: frozenset[str]
    known_quest_flags: frozenset[str]
    items: tuple[ItemDefinition, ...] = ()
    encounters: tuple[Encounter, ...] = ()
    progression_thresholds: tuple[ProgressionThreshold, ...] = ()
    day_events: tuple[DayEvent, ...] = ()
    quest_implications: tuple[QuestImplication, ...] = ()
    text_entries: tuple[tuple[str, str], ...] = ()
    upgrades: tuple[ContentUpgrade, ...] = ()

    def location(self, location_id: str) -> Location:
        for location in self.locations:
            if location.location_id == location_id:
                return location
        raise KeyError(location_id)

    def investigation(self, investigation_id: str) -> Investigation:
        for investigation in self.investigations:
            if investigation.investigation_id == investigation_id:
                return investigation
        raise KeyError(investigation_id)

    def table(self, table_id: str) -> RandomTable:
        for table in self.random_tables:
            if table.table_id == table_id:
                return table
        raise KeyError(table_id)

    def item(self, item_id: str) -> ItemDefinition:
        for item in self.items:
            if item.item_id == item_id:
                return item
        raise KeyError(item_id)

    def encounter(self, encounter_id: str) -> Encounter:
        for encounter in self.encounters:
            if encounter.encounter_id == encounter_id:
                return encounter
        raise KeyError(encounter_id)

    def text(self, text_id: str) -> str:
        for candidate, value in self.text_entries:
            if candidate == text_id:
                return value
        raise KeyError(text_id)


def gate_satisfied(state: SessionState, gate: Gate) -> bool:
    inventory = state.inventory_map()
    return (
        state.lifecycle == gate.lifecycle
        and gate.required_flags.issubset(state.quest_flags)
        and all(inventory.get(item_id, 0) >= quantity for item_id, quantity in gate.required_items)
        and (gate.required_location is None or state.location_id == gate.required_location)
        and state.progression_level >= gate.minimum_level
        and (gate.profile is None or state.selected_content_profile == gate.profile)
    )


def default_campaign_content(version: str | None = None) -> CampaignContent:
    """Load the shipped Standard campaign; an override supports version fixtures."""
    from ..content.loader import load_standard_campaign

    content = load_standard_campaign()
    return replace(content, version=version) if version is not None else content


__all__ = [
    "CampaignContent", "ContentUpgrade", "DayEvent", "Encounter", "Gate",
    "Investigation", "ItemDefinition", "Location", "MechanicsError",
    "ProgressionThreshold", "QuestImplication", "RandomOutcome", "RandomTable",
    "TravelEdge", "default_campaign_content", "gate_satisfied",
]
