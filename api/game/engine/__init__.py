"""Deterministic campaign engine public surface."""
from .campaign import (
    CampaignContent,
    ContentUpgrade,
    Encounter,
    Gate,
    ItemDefinition,
    ProgressionThreshold,
    default_campaign_content,
    gate_satisfied,
)
from .engine import CampaignEngine
from .invariants import (
    InvariantViolation,
    validate_persisted_state,
    validate_serialized_shape,
    validate_state,
    validate_transition,
)
from .random_source import RandomDraw, RandomSource, ScriptedRandomSource, SystemRandomSource
from .upgrades import StateUpgradeRegistry, UpgradeOutcome, upgrade_content, upgrade_state

__all__ = [
    "CampaignContent", "CampaignEngine", "ContentUpgrade", "Encounter", "Gate",
    "InvariantViolation", "ItemDefinition", "ProgressionThreshold",
    "RandomDraw", "RandomSource", "ScriptedRandomSource", "StateUpgradeRegistry",
    "SystemRandomSource", "UpgradeOutcome", "default_campaign_content", "gate_satisfied",
    "upgrade_content", "upgrade_state", "validate_persisted_state", "validate_serialized_shape",
    "validate_state", "validate_transition",
]
