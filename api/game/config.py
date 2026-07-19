"""Validated, immutable game configuration snapshots."""
from __future__ import annotations

from pathlib import Path, PurePath
from threading import RLock
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator, model_validator

try:
    import tomllib
except ModuleNotFoundError:  # Python 3.10
    import tomli as tomllib


class ChannelPair(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    network: str = Field(pattern=r"^[a-z0-9_-]{1,32}$")
    channel: str = Field(pattern=r"^#[^\x00\r\n]{1,63}$")


class ContentPolicy(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    sexual_content: Literal["exclude"] = "exclude"
    drug_references: Literal["exclude", "non_instructional_only"] = "non_instructional_only"
    violence_intensity: Literal["exclude", "non_graphic"] = "non_graphic"
    abusive_language: Literal["exclude", "exclude_targeted"] = "exclude_targeted"
    real_person_content: Literal["exclude"] = "exclude"


class MilestoneConfig(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    eligible_types: tuple[Literal["campaign_completed"], ...] = ("campaign_completed",)
    destinations: tuple[ChannelPair, ...] = ()


class AIRateLimit(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    enabled: bool = True
    requests: int = Field(default=2, ge=1, le=100)
    window_seconds: int = Field(default=600, ge=1, le=86400)
    burst: int = Field(default=1, ge=1, le=100)

    @model_validator(mode="after")
    def burst_not_over_requests(self) -> "AIRateLimit":
        if self.burst > self.requests:
            raise ValueError("burst must not exceed requests")
        return self


class RateLimits(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    ai: AIRateLimit = AIRateLimit()


class StartingInventoryEntry(BaseModel):
    """One ordered, typed starting-inventory entry from operator configuration."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    item_id: str = Field(min_length=1)
    quantity: int = Field(ge=1, le=100000)

    @field_validator("item_id")
    @classmethod
    def item_id_must_not_be_blank(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("item_id must not be blank")
        return value


class CampaignConfig(BaseModel):
    """Immutable operator-owned initial values and campaign bounds."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    starting_location: str = Field(default="haven", pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    starting_health: int = Field(default=20, ge=1, le=100000)
    starting_max_health: int = Field(default=20, ge=1, le=100000)
    starting_currency: int = Field(default=5, ge=0, le=1000000000)
    starting_inventory: tuple[StartingInventoryEntry, ...] = (
        StartingInventoryEntry(item_id="medkit", quantity=1),
    )
    starting_level: int = Field(default=1, ge=1, le=10000)
    starting_experience: int = Field(default=0, ge=0, le=1000000000)
    starting_day: int = Field(default=1, ge=1, le=1000000)
    starting_countdown: int = Field(default=6, ge=1, le=1000000)
    recovery_per_advance: int = Field(default=2, ge=0, le=100000)
    recovery_action_amount: int = Field(default=8, ge=0, le=100000)
    maximum_health: int = Field(default=1000, ge=1, le=100000)
    maximum_currency: int = Field(default=1000000, ge=0, le=1000000000)
    maximum_item_quantity: int = Field(default=99, ge=1, le=100000)
    inventory_capacity: int = Field(default=20, ge=1, le=100000)
    maximum_level: int = Field(default=100, ge=1, le=10000)
    maximum_experience: int = Field(default=1000000, ge=0, le=1000000000)
    post_game_choices: tuple[Literal["status", "credits", "reset"], ...] = ("status", "credits", "reset")

    @field_validator("starting_inventory", mode="before")
    @classmethod
    def accept_legacy_inventory_pairs(cls, value: object) -> object:
        """Keep Python callers compatible while TOML uses arrays of inline tables."""
        if not isinstance(value, (list, tuple)):
            return value
        normalized: list[object] = []
        for entry in value:
            if isinstance(entry, (list, tuple)) and len(entry) == 2:
                normalized.append({"item_id": entry[0], "quantity": entry[1]})
            else:
                normalized.append(entry)
        return normalized

    def inventory_map(self) -> dict[str, int]:
        """Convert ordered validated entries to the engine's canonical map input."""
        return {entry.item_id: entry.quantity for entry in self.starting_inventory}

    @model_validator(mode="after")
    def valid_initial_values(self) -> "CampaignConfig":
        if self.starting_health > self.starting_max_health:
            raise ValueError("starting_health must not exceed starting_max_health")
        if self.starting_max_health > self.maximum_health:
            raise ValueError("starting_max_health must not exceed maximum_health")
        if self.starting_currency > self.maximum_currency:
            raise ValueError("starting_currency must not exceed maximum_currency")
        if self.starting_level > self.maximum_level:
            raise ValueError("starting_level must not exceed maximum_level")
        if self.starting_experience > self.maximum_experience:
            raise ValueError("starting_experience must not exceed maximum_experience")
        item_ids = tuple(entry.item_id for entry in self.starting_inventory)
        if len(set(item_ids)) != len(item_ids):
            raise ValueError("starting_inventory contains duplicate item ids")
        if any(entry.quantity > self.maximum_item_quantity for entry in self.starting_inventory):
            raise ValueError("starting_inventory contains an invalid item or quantity")
        if sum(entry.quantity for entry in self.starting_inventory) > self.inventory_capacity:
            raise ValueError("starting_inventory exceeds inventory_capacity")
        if not self.post_game_choices or len(set(self.post_game_choices)) != len(self.post_game_choices):
            raise ValueError("post_game_choices must be non-empty and unique")
        return self


class BoundaryLimits(BaseModel):
    model_config = ConfigDict(frozen=True)
    max_input_bytes: int = Field(ge=1)
    max_menu_lines: int = Field(ge=1)
    max_choices_per_page: int = Field(ge=1)
    max_narration_bytes: int = Field(ge=1)
    action_timeout_seconds: int = Field(ge=1)

    def stricter(self, other: "BoundaryLimits") -> "BoundaryLimits":
        return BoundaryLimits(**{
            name: min(getattr(self, name), getattr(other, name))
            for name in type(self).model_fields
        })


class GameConfigSnapshot(BaseModel):
    """Complete immutable snapshot; defaults are intentionally conservative."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    enabled: bool = False
    command: str = Field(default="avenger", pattern=r"^[a-z0-9_-]{1,32}$")
    public_title: str = Field(default="Harbor Sentinel", min_length=1, max_length=64)
    pm_enabled: bool = True
    pm_reject_mode: Literal["help", "silent"] = "help"
    channel_play_enabled: bool = False
    channel_handoff_notice: bool = True
    channel_allowlist: tuple[ChannelPair, ...] = ()
    database_path: str = "data/game.db"
    database_busy_timeout_ms: int = Field(default=5000, ge=1, le=60000)
    database_pool_size: int = Field(default=4, ge=1, le=32)
    action_timeout_seconds: int = Field(default=10, ge=1, le=120)
    recovery_timeout_seconds: int = Field(default=30, ge=1, le=300)
    menu_context_ttl_seconds: int = Field(default=900, ge=30, le=86400)
    reset_confirmation_ttl_seconds: int = Field(default=300, ge=30, le=86400)
    max_continuations_per_context: int = Field(default=12, ge=1, le=12)
    max_continuation_identities: int = Field(default=10000, ge=1, le=100000)
    max_input_bytes: int = Field(default=512, ge=1, le=4096)
    max_pending_actions_per_player: int = Field(default=2, ge=1, le=32)
    action_cooldown_ms: int = Field(default=750, ge=1, le=60000)
    action_burst: int = Field(default=4, ge=1, le=100)
    action_window_seconds: int = Field(default=10, ge=1, le=3600)
    max_menu_lines: int = Field(default=4, ge=2, le=20)
    max_choices_per_page: int = Field(default=10, ge=1, le=12)
    page_size: int = Field(default=10, ge=1, le=12)
    max_narration_bytes: int = Field(default=600, ge=1, le=2000)
    standard_content_profile: Literal["standard"] = "standard"
    adult_content_enabled: bool = False
    real_person_content_enabled: bool = False
    ai_enhancement_enabled: bool = False
    milestone_announcements_enabled: bool = False
    save_retention_days: int = Field(default=180, ge=1)
    save_expiry_warning_days: int = Field(default=30, ge=1)
    action_record_retention_days: int = Field(default=90, ge=1)
    reset_archive_retention_days: int = Field(default=30, ge=1)
    recovery_snapshot_retention_days: int = Field(default=30, ge=1)
    audit_retention_days: int = Field(default=365, ge=1)
    maintenance_interval_seconds: int = Field(default=86400, ge=60)
    maintenance_batch_size: int = Field(default=100, ge=1, le=10000)
    backup_enabled: bool = True
    backup_interval_seconds: int = Field(default=86400, ge=60)
    backup_directory: str = "data/backups/game"
    backup_retention_count: int = Field(default=7, ge=1)
    config_revision: int = Field(default=1, ge=1)
    content_policy_revision: int = Field(default=1, ge=1)
    campaign: CampaignConfig = CampaignConfig()
    milestones: MilestoneConfig = MilestoneConfig()
    content_policy: ContentPolicy = ContentPolicy()
    rate_limits: RateLimits = RateLimits()

    @model_validator(mode="after")
    def safe_relationships(self) -> "GameConfigSnapshot":
        _validate_local_data_path(self.database_path, suffix=".db")
        _validate_local_data_path(self.backup_directory)
        if self.page_size > self.max_choices_per_page:
            raise ValueError("page_size must not exceed max_choices_per_page")
        if self.adult_content_enabled or self.real_person_content_enabled:
            raise ValueError("the MVP supports only the restrictive Standard fictionalized content profile")
        if self.save_expiry_warning_days >= self.save_retention_days:
            raise ValueError("save_expiry_warning_days must be less than save_retention_days")
        if self.audit_retention_days < self.action_record_retention_days:
            raise ValueError("audit_retention_days must cover action_record_retention_days")
        pairs = (*self.channel_allowlist, *self.milestones.destinations)
        keys = [(p.network, p.channel.casefold()) for p in pairs]
        if len(keys) != len(set(keys)):
            raise ValueError("duplicate game network/channel pair")
        return self

    def boundary_limits(self) -> BoundaryLimits:
        return BoundaryLimits(
            max_input_bytes=self.max_input_bytes,
            max_menu_lines=self.max_menu_lines,
            max_choices_per_page=self.max_choices_per_page,
            max_narration_bytes=self.max_narration_bytes,
            action_timeout_seconds=self.action_timeout_seconds,
        )


def _validate_local_data_path(value: str, suffix: str = "") -> None:
    path = PurePath(value)
    if not value or path.is_absolute() or "://" in value or "\x00" in value:
        raise ValueError("path must be local and relative")
    if path.parts[0] != "data" or ".." in path.parts or str(path) != value:
        raise ValueError("path must be normalized beneath data/")
    if suffix and not value.endswith(suffix):
        raise ValueError(f"path must end in {suffix}")


class GameConfigStore:
    """Atomic snapshot holder; invalid candidates never replace the prior value."""

    def __init__(self, initial: GameConfigSnapshot | None = None):
        self._lock = RLock()
        self._snapshot = initial or GameConfigSnapshot()

    def snapshot(self) -> GameConfigSnapshot:
        with self._lock:
            return self._snapshot

    def replace(self, candidate: dict) -> GameConfigSnapshot:
        validated = GameConfigSnapshot.model_validate(candidate)
        with self._lock:
            if validated.config_revision <= self._snapshot.config_revision:
                raise ValueError("config_revision must increase")
            self._snapshot = validated
            return validated


def load_game_config(path: Path | str = Path("api/config/game_settings.toml")) -> GameConfigSnapshot:
    config_path = Path(path)
    with config_path.open("rb") as handle:
        document = tomllib.load(handle)
    return GameConfigSnapshot.model_validate(document.get("game", document))


__all__ = [
    "BoundaryLimits",
    "CampaignConfig",
    "GameConfigSnapshot",
    "GameConfigStore",
    "StartingInventoryEntry",
    "ValidationError",
    "load_game_config",
]
