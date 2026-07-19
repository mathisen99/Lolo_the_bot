"""Bounded game-only HTTP contracts shared by action/lifecycle/health routes."""
from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


class ErrorCategory(str, Enum):
    INVALID_INPUT = "invalid_input"
    REQUEST_ID_MISMATCH = "request_id_mismatch"
    IDENTITY_AMBIGUOUS = "identity_ambiguous"
    STALE_REVISION = "stale_revision"
    STALE_CONTEXT = "stale_context"
    IDEMPOTENCY_CONFLICT = "idempotency_conflict"
    CONTENT_UNAVAILABLE = "content_unavailable"
    RECOVERY_REQUIRED = "recovery_required"
    DATABASE_UNAVAILABLE = "database_unavailable"
    DATABASE_BUSY = "database_busy"
    MIGRATION_FAILED = "migration_failed"
    ENGINE_INVARIANT_ERROR = "engine_invariant_error"
    AI_UNAVAILABLE = "ai_unavailable"
    GAME_UNAVAILABLE = "game_unavailable"
    RESPONSE_INVALID = "response_invalid"


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class SessionIdentity(StrictModel):
    kind: Literal["registered_user", "unregistered_nick"]
    value: str = Field(min_length=1, max_length=128)

    @model_validator(mode="after")
    def no_controls(self) -> "SessionIdentity":
        if len(self.value.encode()) > 128 or any(char in self.value for char in "\x00\r\n"):
            raise ValueError("identity value contains controls or exceeds 128 bytes")
        return self


class RequestSource(StrictModel):
    kind: Literal["pm", "channel"]
    channel: str = Field(default="", max_length=64)
    effective_prefix: str = Field(min_length=1, max_length=8)

    @model_validator(mode="after")
    def valid_target(self) -> "RequestSource":
        if self.kind == "pm" and self.channel:
            raise ValueError("PM source channel must be empty")
        if self.kind == "channel" and not self.channel.startswith("#"):
            raise ValueError("channel source must name an IRC channel")
        return self


class ActionArguments(StrictModel):
    target_id: str | None = Field(default=None, pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    item_id: str | None = Field(default=None, pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    destination_id: str | None = Field(default=None, pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    quantity: int | None = Field(default=None, ge=1, le=99)
    page: int | None = Field(default=None, ge=1, le=1000)
    token: str | None = Field(default=None, pattern=r"^(?:r-[a-z2-7]{6,26}|[a-z][a-z0-9_-]{0,31})$")
    text: str | None = None
    fallback: bool = False


ACTION_NAMES = {
    "resume", "start", "status", "inventory", "help", "credits", "reset", "quit",
    "content", "privacy", "delete", "look", "travel", "attack", "defend", "use",
    "equip", "buy", "sell", "escape", "recover", "investigate", "advance", "finalize",
    "next", "prev", "page", "ask",
}


class Action(StrictModel):
    name: str = Field(min_length=1, max_length=32)
    arguments: ActionArguments = Field(default_factory=ActionArguments)
    menu_context_id: str | None = Field(default=None, pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    choice_token: str | None = Field(default=None, pattern=r"^(?:c-[a-z2-7]{6,26}|[a-z][a-z0-9_-]{0,31})$")

    @model_validator(mode="after")
    def action_specific_arguments(self) -> "Action":
        if self.name not in ACTION_NAMES:
            raise ValueError("unsupported action name")
        args = self.arguments
        required = {
            "travel": args.destination_id,
            "attack": args.target_id,
            "use": args.item_id,
            "equip": args.item_id,
        }
        if self.name in required and not required[self.name]:
            raise ValueError(f"{self.name} requires its typed identifier")
        if self.name in {"buy", "sell"} and (not args.item_id or args.quantity is None):
            raise ValueError(f"{self.name} requires item_id and quantity")
        if self.name == "page" and args.page is None:
            raise ValueError("page requires page argument")
        if self.name == "reset" and args.token and not args.token.startswith("r-"):
            raise ValueError("reset requires an r- token")
        if args.fallback and self.name != "help":
            raise ValueError("fallback is accepted only for authored help")
        return self


class ClientContext(StrictModel):
    content_policy_revision: int = Field(ge=1)
    configuration_revision: int = Field(ge=1)


class GameActionRequest(StrictModel):
    request_id: UUID
    idempotency_key: UUID
    network_id: str = Field(pattern=r"^[a-z0-9_-]{1,32}$")
    identity: SessionIdentity
    display_nick: str = Field(min_length=1, max_length=64)
    source: RequestSource
    operation: Literal["action"] = "action"
    mode: Literal["direct", "ai_interpret"] = "direct"
    expected_state_revision: int = Field(ge=0)
    action: Action
    client_context: ClientContext

    @field_validator("display_nick")
    @classmethod
    def bounded_display_nick_bytes(cls, value: str) -> str:
        if len(value.encode()) > 64 or any(char in value for char in "\x00\r\n"):
            raise ValueError("display nick must be 1-64 safe bytes")
        return value

    @model_validator(mode="after")
    def mode_arguments(self) -> "GameActionRequest":
        text = self.action.arguments.text
        if self.mode == "ai_interpret":
            if self.action.name != "ask" or not text:
                raise ValueError("AI mode requires ask with text")
        elif text is not None or self.action.name == "ask":
            raise ValueError("ask text is accepted only in AI mode")
        return self


class Delivery(StrictModel):
    target: Literal["pm", "channel"]
    lines: list[str] = Field(max_length=20)

    @model_validator(mode="after")
    def bounded_lines(self) -> "Delivery":
        if any(len(line.encode()) > 800 or any(c in line for c in "\x00\r\n") for line in self.lines):
            raise ValueError("delivery line is oversized or unsafe")
        return self


class MenuContext(StrictModel):
    id: str = Field(pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    state_revision: int = Field(ge=0)
    page: int = Field(ge=1, le=1000)
    expires_at: datetime


class Continuation(StrictModel):
    input: str = Field(pattern=r"^(?:[a-z][a-z0-9_-]{0,31}|[crm]-[a-z2-7]{6,26})$")
    kind: Literal["action", "choice", "confirmation", "pagination"]
    action: str = Field(min_length=1, max_length=32)
    arguments: ActionArguments = Field(default_factory=ActionArguments)
    choice_token: str = ""
    menu_context_id: str = Field(pattern=r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
    state_revision: int = Field(ge=0)


class StableError(StrictModel):
    category: ErrorCategory
    message: str = Field(min_length=1, max_length=160)
    retryable: bool = False


class GameActionResponse(StrictModel):
    request_id: UUID
    status: Literal["success", "error", "unavailable"]
    result_category: str = Field(min_length=1, max_length=64)
    state_revision: int = Field(ge=0)
    state_changed: bool
    deliveries: list[Delivery] = Field(default_factory=list, max_length=2)
    menu_context: MenuContext | None = None
    continuations: list[Continuation] = Field(default_factory=list, max_length=12)
    milestones: list[str] = Field(default_factory=list, max_length=8)
    error: StableError | None = None

    @model_validator(mode="after")
    def coherent_context(self) -> "GameActionResponse":
        if (self.status == "success") != (self.error is None):
            raise ValueError("success must omit error and failures must include one")
        if self.menu_context and self.menu_context.state_revision != self.state_revision:
            raise ValueError("menu context revision mismatch")
        if self.continuations and self.menu_context is None:
            raise ValueError("continuations require a menu context")
        seen: set[str] = set()
        for continuation in self.continuations:
            if (
                continuation.input in seen
                or continuation.state_revision != self.state_revision
                or self.menu_context is None
                or continuation.menu_context_id != self.menu_context.id
                or continuation.action not in ACTION_NAMES
            ):
                raise ValueError("invalid, duplicate, or mismatched continuation")
            seen.add(continuation.input)
        return self


class LifecycleRequest(StrictModel):
    request_id: UUID
    network_id: str = Field(pattern=r"^[a-z0-9_-]{1,32}$")
    operation: Literal["transfer_identity", "invalidate_context"]
    identity: SessionIdentity
    new_identity: SessionIdentity | None = None
    configuration_revision: int = Field(ge=1)

    @model_validator(mode="after")
    def transfer_target(self) -> "LifecycleRequest":
        if self.operation == "transfer_identity" and self.new_identity is None:
            raise ValueError("transfer_identity requires new_identity")
        return self


class LifecycleResponse(StrictModel):
    request_id: UUID
    status: Literal["success", "error", "unavailable"]
    error: StableError | None = None

    @model_validator(mode="after")
    def coherent_error(self) -> "LifecycleResponse":
        if (self.status == "success") != (self.error is None):
            raise ValueError("success must omit error and failures must include one")
        return self


class GameHealthResponse(StrictModel):
    status: Literal["ready", "disabled", "degraded"]
    database_available: bool
    schema_version: int = Field(ge=0)
    migration_status: Literal["not_started", "not_required", "current", "failed"]
    engine_version: str = Field(max_length=32)
    content_version: str = Field(max_length=64)
    config_revision: int = Field(ge=1)
    ai_status: Literal["disabled", "disabled_missing_credentials", "available", "unavailable"]
    error_category: ErrorCategory | None = None
