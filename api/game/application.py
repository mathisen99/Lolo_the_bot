"""Game-only application orchestration.

The service in this module owns sequencing, not campaign rules.  It snapshots
configuration and content once, normalizes direct input, asks the store to run
one transaction, invokes the engine only inside that transaction, and renders
only the committed authoritative result.  It deliberately has no imports from
the generic command, mention, tool, or AI packages.
"""
from __future__ import annotations

import asyncio
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field
from datetime import datetime, timezone
from threading import RLock
from typing import Any, Literal, Protocol
from uuid import UUID

from .config import GameConfigSnapshot, GameConfigStore
from .observability import GameObserver, GameTelemetry, action_event
from .models.api import (
    ActionArguments,
    Continuation,
    Delivery,
    ErrorCategory,
    GameActionRequest,
    GameActionResponse,
    LifecycleRequest,
    LifecycleResponse,
    MenuContext,
    StableError,
)

ArgumentValue = str | int | bool


@dataclass(frozen=True)
class ContentSnapshot:
    """Small immutable content handle used by orchestration.

    Content loaders added by the content task may keep their validated domain
    objects in ``records``.  A tuple is used so callers cannot mutate a live
    snapshot while an action is running.
    """

    version: str
    profile: str = "standard"
    manifest_hash: str = ""
    records: tuple[tuple[str, object], ...] = ()


class ContentSnapshotStore:
    """Atomic holder for complete, already validated content snapshots."""

    def __init__(self, initial: ContentSnapshot):
        self._lock = RLock()
        self._snapshot = initial

    def snapshot(self) -> ContentSnapshot:
        with self._lock:
            return self._snapshot

    def replace(self, candidate: ContentSnapshot) -> ContentSnapshot:
        if not candidate.version or candidate.profile != "standard":
            raise ValueError("content snapshot requires the Standard profile and a version")
        if candidate.manifest_hash and (
            len(candidate.manifest_hash) != 64
            or any(char not in "0123456789abcdef" for char in candidate.manifest_hash)
        ):
            raise ValueError("content snapshot manifest hash is invalid")
        with self._lock:
            self._snapshot = candidate
            return candidate


@dataclass(frozen=True)
class NormalizedAction:
    """Syntax-normalized input; it carries no authority over campaign state."""

    name: str
    arguments: tuple[tuple[str, ArgumentValue], ...] = ()
    menu_context_id: str | None = None
    choice_token: str | None = None

    def argument_dict(self) -> dict[str, ArgumentValue]:
        return dict(self.arguments)


class DirectActionParser:
    """Normalize the bounded wire action without validating campaign state."""

    def parse(self, request: GameActionRequest) -> NormalizedAction:
        if request.mode != "direct":
            raise GameServiceError(
                ErrorCategory.AI_UNAVAILABLE,
                "AI interpretation is unavailable; use a direct game action.",
                result_category="ai_unavailable",
            )
        values = request.action.arguments.model_dump(exclude_none=True)
        if "text" in values:
            # The wire contract normally catches this.  Keep the application
            # boundary fail-closed if it is called directly in a test/adapter.
            raise GameServiceError(
                ErrorCategory.INVALID_INPUT,
                "Direct actions cannot contain AI text.",
                result_category="invalid_input",
            )
        return NormalizedAction(
            name=request.action.name,
            arguments=tuple(sorted(values.items())),
            menu_context_id=request.action.menu_context_id,
            choice_token=request.action.choice_token,
        )


@dataclass(frozen=True)
class EngineContext:
    config: GameConfigSnapshot
    content: ContentSnapshot
    random: object | None = None
    now: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    identity: tuple[str, str, str] | None = None


@dataclass(frozen=True)
class StoreActionRequest:
    request_id: UUID
    idempotency_key: UUID
    network_id: str
    identity_kind: str
    identity_value: str
    expected_state_revision: int
    action: NormalizedAction
    configuration_revision: int
    content_policy_revision: int
    display_nick: str = "player"
    engine_version: str = "1"
    content_version: str = "1"
    state_schema_version: int = 1


@dataclass(frozen=True)
class AuthoritativeChoice:
    input: str
    kind: Literal["action", "choice", "confirmation", "pagination"]
    action: str
    arguments: tuple[tuple[str, ArgumentValue], ...] = ()
    choice_token: str = ""


@dataclass(frozen=True)
class AuthoritativeResult:
    """Committed engine output accepted as the sole source for rendering."""

    result_category: str
    state_revision: int
    state_changed: bool
    facts: tuple[str, ...]
    choices: tuple[AuthoritativeChoice, ...] = ()
    menu_context_id: str | None = None
    menu_page: int = 1
    menu_expires_at: Any | None = None
    milestones: tuple[str, ...] = ()
    delivery_target: Literal["pm", "channel"] = "pm"
    next_state: object | None = field(default=None, repr=False, compare=False)
    random_metadata: tuple[tuple[str, object], ...] = ()

    def __post_init__(self) -> None:
        if self.state_revision < 0:
            raise ValueError("state revision cannot be negative")
        context_fields = (self.menu_context_id, self.menu_expires_at)
        if self.choices and any(value is None for value in context_fields):
            raise ValueError("choices require a complete menu context")
        if not self.choices and any(value is not None for value in context_fields):
            raise ValueError("menu context requires choices")


class GameEngine(Protocol):
    def transition(
        self,
        state: object | None,
        action: NormalizedAction,
        context: EngineContext,
    ) -> AuthoritativeResult: ...


class GameStore(Protocol):
    def execute(
        self,
        request: StoreActionRequest,
        transition: Callable[[object | None], AuthoritativeResult],
    ) -> AuthoritativeResult: ...

    def execute_lifecycle(self, request: LifecycleRequest) -> None: ...

    def run_maintenance(self, now: datetime | None = None) -> object: ...


class ActionRenderer(Protocol):
    def render(
        self,
        request: GameActionRequest,
        result: AuthoritativeResult,
        config: GameConfigSnapshot,
        content: ContentSnapshot,
    ) -> GameActionResponse: ...


class AIActionAdapter(Protocol):
    def propose(self, request: GameActionRequest) -> NormalizedAction: ...


class GameServiceError(Exception):
    """Expected application failure represented by a stable public category."""

    def __init__(
        self,
        category: ErrorCategory,
        message: str,
        *,
        result_category: str | None = None,
        retryable: bool = False,
        state_revision: int = 0,
    ) -> None:
        super().__init__(message)
        self.category = category
        self.public_message = message
        self.result_category = result_category or category.value
        self.retryable = retryable
        self.state_revision = state_revision


class AuthoritativeRenderer:
    """Render bounded facts and navigable continuations from authoritative data."""

    @staticmethod
    def _safe_line(value: str, maximum_bytes: int) -> str:
        # Python emits plain text only. Go remains responsible for supported
        # formatting tags, network byte splitting, and outbound queue policy.
        cleaned = "".join(" " if ord(char) < 32 or ord(char) == 127 else char for char in value)
        cleaned = " ".join(cleaned.split())
        encoded = cleaned.encode("utf-8")
        maximum_bytes = min(maximum_bytes, 800)
        if len(encoded) <= maximum_bytes:
            return cleaned
        shortened = encoded[:maximum_bytes]
        while shortened:
            try:
                return shortened.decode("utf-8").rstrip()
            except UnicodeDecodeError:
                shortened = shortened[:-1]
        return ""

    @staticmethod
    def _choice_label(choice: AuthoritativeChoice) -> str:
        arguments = dict(choice.arguments)
        if choice.kind == "confirmation":
            return f"reset {choice.input}"
        label = choice.action
        if "destination_id" in arguments:
            label += f" {arguments['destination_id']}"
        elif "target_id" in arguments:
            label += f" {arguments['target_id']}"
        elif "item_id" in arguments:
            quantity = arguments.get("quantity")
            label += f" {arguments['item_id']}"
            if quantity is not None:
                label += f" {quantity}"
        return label

    def render(
        self,
        request: GameActionRequest,
        result: AuthoritativeResult,
        config: GameConfigSnapshot,
        content: ContentSnapshot,
    ) -> GameActionResponse:
        del content  # Versioned content was already consumed by the engine.
        safe_facts = [self._safe_line(fact, config.max_narration_bytes) for fact in result.facts]
        safe_facts = [fact for fact in safe_facts if fact]

        menu_context = None
        continuations: list[Continuation] = []
        menu_line = ""
        if result.choices:
            assert result.menu_context_id is not None and result.menu_expires_at is not None
            page_size = min(config.page_size, config.max_choices_per_page)
            total_pages = max(1, (len(result.choices) + page_size - 1) // page_size)
            if result.menu_page < 1 or result.menu_page > total_pages:
                raise GameServiceError(
                    ErrorCategory.INVALID_INPUT,
                    "That menu page is unavailable; use the current menu.",
                    result_category="invalid_page",
                    state_revision=result.state_revision,
                )
            start = (result.menu_page - 1) * page_size
            visible = result.choices[start:start + page_size]
            menu_context = MenuContext(
                id=result.menu_context_id,
                state_revision=result.state_revision,
                page=result.menu_page,
                expires_at=result.menu_expires_at,
            )
            labels: list[str] = []
            for choice in visible:
                labels.append(self._choice_label(choice))
                continuations.append(Continuation(
                    input=choice.input,
                    kind=choice.kind,
                    action=choice.action,
                    arguments=ActionArguments.model_validate(dict(choice.arguments)),
                    choice_token=choice.choice_token,
                    menu_context_id=result.menu_context_id,
                    state_revision=result.state_revision,
                ))
            navigation: list[str] = []
            if result.menu_page > 1:
                navigation.append("prev")
                continuations.append(Continuation(
                    input="prev", kind="pagination", action="page",
                    arguments=ActionArguments(page=result.menu_page - 1),
                    menu_context_id=result.menu_context_id,
                    state_revision=result.state_revision,
                ))
            if result.menu_page < total_pages:
                navigation.append("next")
                continuations.append(Continuation(
                    input="next", kind="pagination", action="page",
                    arguments=ActionArguments(page=result.menu_page + 1),
                    menu_context_id=result.menu_context_id,
                    state_revision=result.state_revision,
                ))
            if len(continuations) > config.max_continuations_per_context:
                raise GameServiceError(
                    ErrorCategory.RESPONSE_INVALID,
                    "The current menu is too large to display safely.",
                    result_category="response_invalid",
                    state_revision=result.state_revision,
                )
            heading = "Choices: "
            if total_pages > 1:
                if navigation == ["next"]:
                    instruction = "reply next for more"
                elif navigation == ["prev"]:
                    instruction = "reply prev to go back"
                else:
                    instruction = "reply prev or next"
                heading = f"Choices (page {result.menu_page} of {total_pages}; {instruction}): "
            menu_line = self._safe_line(
                heading + ", ".join(labels),
                config.max_narration_bytes,
            )

        lines: list[str] = []
        if safe_facts:
            # Keep every authoritative fact while reserving a final line for
            # actionable choices. Go may split this logical line per network.
            lines.append(self._safe_line(" ".join(safe_facts), config.max_narration_bytes))
        if menu_line:
            lines.append(menu_line)
        lines = [line for line in lines if line][: config.max_menu_lines]
        if not lines:
            raise GameServiceError(
                ErrorCategory.RESPONSE_INVALID,
                "The game produced no safe response. Contact an operator with the Request ID.",
                result_category="response_invalid",
                state_revision=result.state_revision,
            )

        return GameActionResponse(
            request_id=request.request_id,
            status="success",
            result_category=result.result_category,
            state_revision=result.state_revision,
            state_changed=result.state_changed,
            deliveries=[Delivery(target=result.delivery_target, lines=lines)],
            menu_context=menu_context,
            continuations=continuations,
            milestones=list(result.milestones),
        )


class GameService:
    """Coordinate one authoritative game action across isolated components."""

    def __init__(
        self,
        *,
        configs: GameConfigStore,
        contents: ContentSnapshotStore,
        parser: DirectActionParser,
        store: GameStore,
        engine: GameEngine,
        renderer: ActionRenderer,
        ai_adapter_factory: Callable[[], AIActionAdapter] | None = None,
        random_source_factory: Callable[[], object] | None = None,
        now_factory: Callable[[], datetime] | None = None,
        observer: GameObserver | None = None,
    ) -> None:
        self._configs = configs
        self._contents = contents
        self._parser = parser
        self._store = store
        self._engine = engine
        self._renderer = renderer
        self._ai_adapter_factory = ai_adapter_factory
        if random_source_factory is None:
            from .engine.random_source import SystemRandomSource
            random_source_factory = SystemRandomSource
        self._random_source_factory = random_source_factory
        self._now_factory = now_factory or (lambda: datetime.now(timezone.utc))
        self._observer = observer or GameTelemetry()

    def _resolve_action(self, request: GameActionRequest) -> NormalizedAction:
        if request.mode == "direct":
            # Do not even construct an AI adapter on the direct path.
            return self._parser.parse(request)
        if self._ai_adapter_factory is None:
            raise GameServiceError(
                ErrorCategory.AI_UNAVAILABLE,
                "AI interpretation is unavailable; use a direct game action.",
                result_category="ai_unavailable",
            )
        return self._ai_adapter_factory().propose(request)

    async def handle_action(self, request: GameActionRequest) -> GameActionResponse:
        config = self._configs.snapshot()
        content = self._contents.snapshot()
        started = time.perf_counter()
        response: GameActionResponse
        try:
            action = self._resolve_action(request)
            store_request = StoreActionRequest(
                request_id=request.request_id,
                idempotency_key=request.idempotency_key,
                network_id=request.network_id,
                identity_kind=request.identity.kind,
                identity_value=request.identity.value,
                expected_state_revision=request.expected_state_revision,
                action=action,
                configuration_revision=config.config_revision,
                content_policy_revision=config.content_policy_revision,
                display_nick=request.display_nick,
                engine_version=str(getattr(self._engine, "version", "1")),
                content_version=content.version,
                state_schema_version=1,
            )
            context = EngineContext(
                config=config,
                content=content,
                random=self._random_source_factory(),
                now=self._now_factory(),
                identity=(request.network_id, request.identity.kind, request.identity.value),
            )

            def transition(state: object | None) -> AuthoritativeResult:
                # This is the only campaign-state validation/mutation call in
                # the application layer, and the store invokes it transactionally.
                return self._engine.transition(state, action, context)

            committed = await asyncio.to_thread(self._store.execute, store_request, transition)
            response = self._renderer.render(request, committed, config, content)
            if response.request_id != request.request_id:
                raise GameServiceError(
                    ErrorCategory.RESPONSE_INVALID,
                    "The game returned an invalid response. Contact an operator with the Request ID.",
                    result_category="response_invalid",
                    state_revision=committed.state_revision,
                )
        except GameServiceError as exc:
            response = self._error_response(request, exc)
        except Exception:
            # Raw store/engine/renderer exceptions never cross the API boundary.
            response = self._error_response(request, GameServiceError(
                ErrorCategory.GAME_UNAVAILABLE,
                "Game processing is temporarily unavailable.",
                result_category="game_unavailable",
                retryable=True,
            ))
        self._observe_action(request, response, config, started)
        return response

    def _observe_action(
        self,
        request: GameActionRequest,
        response: GameActionResponse,
        config: GameConfigSnapshot,
        started: float,
    ) -> None:
        try:
            self._observer.observe(action_event(
                request_id=request.request_id,
                network_id=request.network_id,
                identity_kind=request.identity.kind,
                identity_value=request.identity.value,
                action_type=request.action.name,
                pre_revision=request.expected_state_revision,
                post_revision=response.state_revision,
                latency_ms=int((time.perf_counter() - started) * 1000),
                result_category=response.result_category,
                error_category=response.error.category.value if response.error else None,
                configuration_revision=config.config_revision,
                content_policy_revision=config.content_policy_revision,
            ))
        except Exception:
            # Telemetry must never change a committed result or block gameplay.
            pass

    async def run_maintenance(self, now: datetime | None = None) -> object:
        run = getattr(self._store, "run_maintenance", None)
        if run is None:
            return None
        return await asyncio.to_thread(run, now)

    async def handle_lifecycle(self, request: LifecycleRequest) -> LifecycleResponse:
        try:
            await asyncio.to_thread(self._store.execute_lifecycle, request)
            return LifecycleResponse(request_id=request.request_id, status="success")
        except GameServiceError as exc:
            return LifecycleResponse(
                request_id=request.request_id,
                status="unavailable" if exc.retryable else "error",
                error=StableError(
                    category=exc.category,
                    message=exc.public_message,
                    retryable=exc.retryable,
                ),
            )
        except Exception:
            return LifecycleResponse(
                request_id=request.request_id,
                status="unavailable",
                error=StableError(
                    category=ErrorCategory.GAME_UNAVAILABLE,
                    message="Game lifecycle is temporarily unavailable.",
                    retryable=True,
                ),
            )

    def close(self) -> None:
        """Close game-owned persistence without affecting unrelated API services."""
        close = getattr(self._store, "close", None)
        if close is not None:
            close()

    @staticmethod
    def _error_response(request: GameActionRequest, error: GameServiceError) -> GameActionResponse:
        return GameActionResponse(
            request_id=request.request_id,
            status="unavailable" if error.retryable else "error",
            result_category=error.result_category,
            state_revision=error.state_revision,
            state_changed=False,
            deliveries=[Delivery(target="pm", lines=[error.public_message])],
            error=StableError(
                category=error.category,
                message=error.public_message,
                retryable=error.retryable,
            ),
        )


__all__ = [
    "ActionRenderer",
    "AIActionAdapter",
    "AuthoritativeChoice",
    "AuthoritativeRenderer",
    "AuthoritativeResult",
    "ContentSnapshot",
    "ContentSnapshotStore",
    "DirectActionParser",
    "EngineContext",
    "GameEngine",
    "GameService",
    "GameServiceError",
    "GameStore",
    "NormalizedAction",
    "StoreActionRequest",
]
