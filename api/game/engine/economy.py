"""Pure atomic economy, inventory, equipment, and consumable transitions."""
from __future__ import annotations

from dataclasses import dataclass, replace

from ..config import CampaignConfig
from ..models.domain import SessionState, frozen_equipment, frozen_items
from .campaign import CampaignContent, ItemDefinition, MechanicsError


@dataclass(frozen=True)
class EconomyTransition:
    state: SessionState
    category: str
    facts: tuple[str, ...]


def inventory_load(state: SessionState, content: CampaignContent) -> int:
    return sum(content.item(item_id).capacity_cost * quantity for item_id, quantity in state.inventory)


def add_item_bounded(
    state: SessionState,
    item: ItemDefinition,
    quantity: int,
    config: CampaignConfig,
    content: CampaignContent,
) -> tuple[tuple[tuple[str, int], ...], int]:
    """Add as much as configured quantity/capacity bounds permit."""
    if quantity <= 0:
        return state.inventory, 0
    inventory = state.inventory_map()
    per_item_room = max(0, config.maximum_item_quantity - inventory.get(item.item_id, 0))
    capacity_room = max(0, config.inventory_capacity - inventory_load(state, content))
    capacity_quantity = capacity_room // item.capacity_cost
    added = min(quantity, per_item_room, capacity_quantity)
    if added:
        inventory[item.item_id] = inventory.get(item.item_id, 0) + added
    return frozen_items(inventory), added


def buy(
    state: SessionState,
    item_id: object,
    quantity: object,
    config: CampaignConfig,
    content: CampaignContent,
) -> EconomyTransition:
    if not content.location(state.location_id).commerce_allowed:
        raise MechanicsError("Buying is available only at a commerce location.")
    item = _item(content, item_id)
    amount = _quantity(quantity)
    total = item.price * amount
    if state.currency < total:
        raise MechanicsError("There is not enough currency for that purchase.")
    inventory, added = add_item_bounded(state, item, amount, config, content)
    if added != amount:
        raise MechanicsError("The inventory has insufficient capacity for that purchase.")
    return EconomyTransition(
        replace(state, inventory=inventory, currency=state.currency - total),
        "item_bought",
        (f"Bought {item.item_id} x{amount} for {total} currency.",),
    )


def sell(
    state: SessionState,
    item_id: object,
    quantity: object,
    content: CampaignContent,
) -> EconomyTransition:
    if not content.location(state.location_id).commerce_allowed:
        raise MechanicsError("Selling is available only at a commerce location.")
    item = _item(content, item_id)
    amount = _quantity(quantity)
    inventory = state.inventory_map()
    current = inventory.get(item.item_id, 0)
    if current < amount:
        raise MechanicsError("The inventory does not contain that quantity.")
    remaining = current - amount
    if item.item_id in state.equipped_map().values() and remaining < 1:
        raise MechanicsError("An equipped item must be unequipped before selling the last copy.")
    if remaining:
        inventory[item.item_id] = remaining
    else:
        inventory.pop(item.item_id, None)
    proceeds = item.sell_price * amount
    return EconomyTransition(
        replace(state, inventory=frozen_items(inventory), currency=state.currency + proceeds),
        "item_sold",
        (f"Sold {item.item_id} x{amount} for {proceeds} currency.",),
    )


def equip(state: SessionState, item_id: object, content: CampaignContent) -> EconomyTransition:
    item = _item(content, item_id)
    if item.equipment_slot is None:
        raise MechanicsError("That item cannot be equipped.")
    if state.inventory_map().get(item.item_id, 0) < 1:
        raise MechanicsError("That item is not in the inventory.")
    equipment = state.equipped_map()
    if equipment.get(item.equipment_slot) == item.item_id:
        raise MechanicsError("That item is already equipped.")
    equipment[item.equipment_slot] = item.item_id
    return EconomyTransition(
        replace(state, equipped=frozen_equipment(equipment)),
        "item_equipped",
        (f"Equipped {item.item_id} in {item.equipment_slot}.",),
    )


def use_consumable(
    state: SessionState,
    item_id: object,
    content: CampaignContent,
) -> EconomyTransition:
    item = _item(content, item_id)
    if item.recovery_amount <= 0:
        raise MechanicsError("That item is not a recovery consumable.")
    inventory = state.inventory_map()
    if inventory.get(item.item_id, 0) < 1:
        raise MechanicsError("That consumable is not in the inventory.")
    if state.health >= state.max_health:
        raise MechanicsError("Health is already full.")
    remaining = inventory[item.item_id] - 1
    if remaining:
        inventory[item.item_id] = remaining
    else:
        del inventory[item.item_id]
    health = min(state.max_health, state.health + item.recovery_amount)
    combat = state.combat
    if combat is not None:
        combat = replace(combat, consumables_used=(*combat.consumables_used, item.item_id))
    return EconomyTransition(
        replace(state, health=health, inventory=frozen_items(inventory), combat=combat),
        "item_used",
        (f"Used {item.item_id}; health is {health}/{state.max_health}.",),
    )


def _item(content: CampaignContent, item_id: object) -> ItemDefinition:
    if not isinstance(item_id, str):
        raise MechanicsError("The action requires an item.")
    try:
        return content.item(item_id)
    except KeyError as exc:
        raise MechanicsError("That item is not available.") from exc


def _quantity(value: object) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 1:
        raise MechanicsError("Quantity must be a positive integer.")
    return value


__all__ = [
    "EconomyTransition", "add_item_bounded", "buy", "equip", "inventory_load",
    "sell", "use_consumable",
]
