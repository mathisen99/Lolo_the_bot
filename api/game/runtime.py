"""Failure-isolated game startup, service installation, and readiness state."""
from __future__ import annotations

import inspect
import os
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from threading import RLock
from typing import Protocol

from .config import GameConfigSnapshot, GameConfigStore, load_game_config
from .models.api import (
    ErrorCategory,
    GameActionRequest,
    GameActionResponse,
    GameHealthResponse,
    LifecycleRequest,
    LifecycleResponse,
    StableError,
)
from .store.migrations import MigrationError
from .store.sqlite import GameDatabaseError


class GameServiceBoundary(Protocol):
    """Narrow boundary with no generic command, mention, or tool dependency."""

    async def handle_action(self, request: GameActionRequest) -> GameActionResponse: ...
    async def handle_lifecycle(self, request: LifecycleRequest) -> LifecycleResponse: ...
    async def run_maintenance(self, now=None) -> object: ...


@dataclass(frozen=True)
class GameStartupResult:
    """Complete successful bootstrap result installed in one atomic step."""

    service: GameServiceBoundary
    schema_version: int
    engine_version: str
    content_version: str
    ai_status: str | None = None


class GameStartupFailure(Exception):
    """Operator-diagnostic startup failure projected to redacted health fields."""

    def __init__(
        self,
        category: ErrorCategory,
        *,
        database_available: bool,
        migration_status: str,
        schema_version: int = 0,
        engine_version: str = "unavailable",
        content_version: str = "unavailable",
    ) -> None:
        super().__init__(category.value)
        self.category = category
        self.database_available = database_available
        self.migration_status = migration_status
        self.schema_version = schema_version
        self.engine_version = engine_version
        self.content_version = content_version


class ContentStartupFailure(GameStartupFailure):
    def __init__(self) -> None:
        super().__init__(
            ErrorCategory.CONTENT_UNAVAILABLE,
            database_available=False,
            migration_status="not_started",
        )


class DatabaseStartupFailure(GameStartupFailure):
    def __init__(self) -> None:
        super().__init__(
            ErrorCategory.DATABASE_UNAVAILABLE,
            database_available=False,
            migration_status="not_started",
        )


class MigrationStartupFailure(GameStartupFailure):
    def __init__(self, *, schema_version: int = 0) -> None:
        super().__init__(
            ErrorCategory.MIGRATION_FAILED,
            database_available=True,
            migration_status="failed",
            schema_version=schema_version,
        )


Initializer = Callable[[GameConfigSnapshot], GameStartupResult]


class GameRuntime:
    def __init__(self) -> None:
        self._lock = RLock()
        self._configs = GameConfigStore()
        self._health = self._disabled_health(self._configs.snapshot())
        self._service: GameServiceBoundary | None = None

    @staticmethod
    def _disabled_health(config: GameConfigSnapshot) -> GameHealthResponse:
        return GameHealthResponse(
            status="disabled",
            database_available=False,
            schema_version=0,
            migration_status="not_required",
            engine_version="unavailable",
            content_version="unavailable",
            config_revision=config.config_revision,
            ai_status="disabled",
        )

    @staticmethod
    def _ai_status(config: GameConfigSnapshot, startup: GameStartupResult) -> str:
        if not config.ai_enhancement_enabled:
            return "disabled"
        if not os.getenv("OPENAI_API_KEY"):
            return "disabled_missing_credentials"
        # The optional adapter owns provider readiness.  Credentials alone do
        # not require general AI initialization on the direct startup path.
        return startup.ai_status or "unavailable"

    def start(
        self,
        config_path: Path | str = Path("api/config/game_settings.toml"),
        initializer: Initializer | None = None,
    ) -> None:
        """Load and initialize without allowing failures to escape FastAPI.

        Readiness is published together with the corresponding service.  A
        caller can therefore never observe ``ready`` with a missing service.
        """
        service: GameServiceBoundary | None = None
        try:
            config = load_game_config(config_path)
            configs = GameConfigStore(config)
            if not config.enabled:
                health = self._disabled_health(config)
            else:
                startup = (initializer or self._default_components)(config)
                if startup.service is None:
                    raise ContentStartupFailure()
                service = startup.service
                health = GameHealthResponse(
                    status="ready",
                    database_available=True,
                    schema_version=startup.schema_version,
                    migration_status="current",
                    engine_version=startup.engine_version,
                    content_version=startup.content_version,
                    config_revision=config.config_revision,
                    ai_status=self._ai_status(config, startup),
                )
        except GameStartupFailure as exc:
            configs = locals().get("configs", self._configs)
            config = configs.snapshot()
            health = GameHealthResponse(
                status="degraded",
                database_available=exc.database_available,
                schema_version=exc.schema_version,
                migration_status=exc.migration_status,
                engine_version=exc.engine_version,
                content_version=exc.content_version,
                config_revision=config.config_revision,
                ai_status=(
                    "disabled_missing_credentials"
                    if config.ai_enhancement_enabled and not os.getenv("OPENAI_API_KEY")
                    else "disabled"
                ),
                error_category=exc.category,
            )
        except MigrationError as exc:
            configs = locals().get("configs", self._configs)
            config = configs.snapshot()
            health = GameHealthResponse(
                status="degraded",
                database_available=True,
                schema_version=exc.version,
                migration_status="failed",
                engine_version="unavailable",
                content_version="unavailable",
                config_revision=config.config_revision,
                ai_status="disabled",
                error_category=ErrorCategory.MIGRATION_FAILED,
            )
        except GameDatabaseError:
            configs = locals().get("configs", self._configs)
            config = configs.snapshot()
            health = GameHealthResponse(
                status="degraded",
                database_available=False,
                schema_version=0,
                migration_status="not_started",
                engine_version="unavailable",
                content_version="unavailable",
                config_revision=config.config_revision,
                ai_status="disabled",
                error_category=ErrorCategory.DATABASE_UNAVAILABLE,
            )
        except Exception:
            # Invalid configuration and unexpected bootstrap errors are kept
            # game-local and never expose paths, SQL, credentials, or messages.
            configs = locals().get("configs", self._configs)
            config = configs.snapshot()
            health = GameHealthResponse(
                status="degraded",
                database_available=False,
                schema_version=0,
                migration_status="not_started",
                engine_version="unavailable",
                content_version="unavailable",
                config_revision=config.config_revision,
                ai_status="disabled",
                error_category=ErrorCategory.GAME_UNAVAILABLE,
            )
        with self._lock:
            self._configs = configs
            self._service = service
            self._health = health

    @staticmethod
    def _default_components(config: GameConfigSnapshot) -> GameStartupResult:
        """Compose the validated Standard corpus with the existing engine/store path."""
        from .application import (
            AuthoritativeRenderer,
            ContentSnapshotStore,
            DirectActionParser,
            GameService,
        )
        from .content import ContentValidationError, load_standard_snapshot
        from .engine import CampaignEngine, validate_persisted_state
        from .store import GameStore

        try:
            content = load_standard_snapshot()
        except ContentValidationError as exc:
            raise ContentStartupFailure() from exc
        campaign = dict(content.records).get("campaign")
        if campaign is None:
            raise ContentStartupFailure()
        try:
            campaign.location(config.campaign.starting_location)
            for entry in config.campaign.starting_inventory:
                campaign.item(entry.item_id)
        except KeyError as exc:
            raise ContentStartupFailure() from exc
        validator = lambda value, revision: validate_persisted_state(
            value, revision, config.campaign, campaign,
        )
        store = GameStore.open(config, invariant_validator=validator)
        service = GameService(
            configs=GameConfigStore(config),
            contents=ContentSnapshotStore(content),
            parser=DirectActionParser(),
            store=store,
            engine=CampaignEngine(),
            renderer=AuthoritativeRenderer(),
        )
        return GameStartupResult(
            service=service,
            schema_version=store.schema_version,
            engine_version=CampaignEngine.version,
            content_version=content.version,
        )

    def config(self) -> GameConfigSnapshot:
        return self._configs.snapshot()

    def health(self) -> GameHealthResponse:
        with self._lock:
            return self._health.model_copy(deep=True)

    def ready(self) -> bool:
        with self._lock:
            return self._health.status == "ready" and self._service is not None

    def install_service(self, service: GameServiceBoundary | None) -> None:
        """Testing/upgrade seam; normal startup uses ``GameStartupResult``."""
        with self._lock:
            self._service = service
            if service is None and self._health.status == "ready":
                self._health = self._health.model_copy(update={
                    "status": "degraded",
                    "error_category": ErrorCategory.GAME_UNAVAILABLE,
                })

    async def stop(self) -> None:
        """Detach and close only game-owned resources."""
        with self._lock:
            service = self._service
            self._service = None
        close = getattr(service, "close", None)
        if close is not None:
            result = close()
            if inspect.isawaitable(result):
                await result

    async def handle_action(self, request: GameActionRequest) -> GameActionResponse:
        with self._lock:
            service = self._service
            ready = self._health.status == "ready" and service is not None
        if not ready:
            return GameActionResponse(
                request_id=request.request_id,
                status="unavailable",
                result_category="game_unavailable",
                state_revision=0,
                state_changed=False,
                error=StableError(
                    category=ErrorCategory.GAME_UNAVAILABLE,
                    message="Game is temporarily unavailable.",
                    retryable=True,
                ),
            )
        return await service.handle_action(request)

    async def run_maintenance(self) -> object:
        """Run one game-local cleanup; failures never affect unrelated API work."""
        with self._lock:
            service = self._service
            ready = self._health.status == "ready" and service is not None
        if not ready:
            return None
        run = getattr(service, "run_maintenance", None)
        if run is None:
            return None
        try:
            return await run()
        except Exception:
            return None

    async def handle_lifecycle(self, request: LifecycleRequest) -> LifecycleResponse:
        with self._lock:
            service = self._service
            ready = self._health.status == "ready" and service is not None
        if not ready:
            return LifecycleResponse(
                request_id=request.request_id,
                status="unavailable",
                error=StableError(
                    category=ErrorCategory.GAME_UNAVAILABLE,
                    message="Game lifecycle is temporarily unavailable.",
                    retryable=True,
                ),
            )
        return await service.handle_lifecycle(request)


game_runtime = GameRuntime()


__all__ = [
    "ContentStartupFailure",
    "DatabaseStartupFailure",
    "GameRuntime",
    "GameServiceBoundary",
    "GameStartupFailure",
    "GameStartupResult",
    "MigrationStartupFailure",
    "game_runtime",
]
