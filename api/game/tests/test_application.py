from __future__ import annotations

import asyncio
from datetime import datetime, timedelta, timezone

import pytest
from pydantic import ValidationError

from api.game.application import (
    AuthoritativeChoice,
    AuthoritativeRenderer,
    AuthoritativeResult,
    ContentSnapshot,
    ContentSnapshotStore,
    DirectActionParser,
    GameService,
    GameServiceError,
)
from api.game.config import GameConfigSnapshot, GameConfigStore
from api.game.models.api import ErrorCategory, GameActionRequest, LifecycleRequest

from .test_config_contracts import valid_request


class RecordingEngine:
    def __init__(self) -> None:
        self.calls = []

    def transition(self, state, action, context):
        self.calls.append((state, action, context))
        return AuthoritativeResult(
            result_category="campaign_started",
            state_revision=1,
            state_changed=True,
            facts=("Campaign started. Health: 20/20.",),
            choices=(
                AuthoritativeChoice(input="look", kind="action", action="look"),
                AuthoritativeChoice(
                    input="c-abcdef",
                    kind="choice",
                    action="travel",
                    arguments=(("destination_id", "harbor"),),
                    choice_token="c-abcdef",
                ),
            ),
            menu_context_id="m-test-context",
            menu_expires_at=datetime.now(timezone.utc) + timedelta(minutes=15),
        )


class TransactionalStore:
    def __init__(self) -> None:
        self.calls = []
        self.lifecycle = []

    def execute(self, request, transition):
        self.calls.append(request)
        # The engine is reached only through this transaction callback.
        result = transition({"opaque": "pre-state"})
        self.calls.append(result)
        return result

    def execute_lifecycle(self, request):
        self.lifecycle.append(request)


def test_game_service_snapshots_transacts_renders_and_never_constructs_ai() -> None:
    config = GameConfigSnapshot(enabled=True, config_revision=7, content_policy_revision=3)
    configs = GameConfigStore(config)
    content = ContentSnapshot(version="standard-test-v1")
    contents = ContentSnapshotStore(content)
    store = TransactionalStore()
    engine = RecordingEngine()

    def forbidden_ai_factory():
        raise AssertionError("direct mode must not initialize AI")

    service = GameService(
        configs=configs,
        contents=contents,
        parser=DirectActionParser(),
        store=store,
        engine=engine,
        renderer=AuthoritativeRenderer(),
        ai_adapter_factory=forbidden_ai_factory,
    )
    request = GameActionRequest.model_validate(valid_request())
    response = asyncio.run(service.handle_action(request))

    assert response.status == "success"
    assert response.state_revision == 1
    assert response.state_changed
    assert response.deliveries[0].lines == [
        "Campaign started. Health: 20/20.",
        "Choices: look, travel harbor",
    ]
    assert [item.input for item in response.continuations] == ["look", "c-abcdef"]
    assert len(store.calls) == 2
    assert store.calls[0].configuration_revision == 7
    assert store.calls[0].content_policy_revision == 3
    assert store.calls[0].action.name == "start"
    assert len(engine.calls) == 1
    assert engine.calls[0][0] == {"opaque": "pre-state"}
    assert engine.calls[0][2].config is config
    assert engine.calls[0][2].content is content

    with pytest.raises(ValidationError):
        config.enabled = False
    with pytest.raises(Exception):
        content.version = "changed"


def test_service_returns_stable_errors_without_leaking_exception_text() -> None:
    class BusyStore(TransactionalStore):
        def execute(self, request, transition):
            raise GameServiceError(
                ErrorCategory.DATABASE_BUSY,
                "Game storage is busy; retry shortly.",
                retryable=True,
            )

    service = GameService(
        configs=GameConfigStore(GameConfigSnapshot(enabled=True)),
        contents=ContentSnapshotStore(ContentSnapshot(version="test")),
        parser=DirectActionParser(),
        store=BusyStore(),
        engine=RecordingEngine(),
        renderer=AuthoritativeRenderer(),
    )
    response = asyncio.run(service.handle_action(GameActionRequest.model_validate(valid_request())))
    payload = response.model_dump(mode="json")
    assert payload["status"] == "unavailable"
    assert payload["error"] == {
        "category": "database_busy",
        "message": "Game storage is busy; retry shortly.",
        "retryable": True,
    }
    assert "traceback" not in str(payload).lower()


def test_lifecycle_uses_game_store_only() -> None:
    store = TransactionalStore()
    service = GameService(
        configs=GameConfigStore(GameConfigSnapshot(enabled=True)),
        contents=ContentSnapshotStore(ContentSnapshot(version="test")),
        parser=DirectActionParser(),
        store=store,
        engine=RecordingEngine(),
        renderer=AuthoritativeRenderer(),
    )
    action = valid_request()
    request = LifecycleRequest.model_validate({
        "request_id": action["request_id"],
        "network_id": "libera",
        "operation": "invalidate_context",
        "identity": {"kind": "unregistered_nick", "value": "alice"},
        "configuration_revision": 1,
    })
    response = asyncio.run(service.handle_lifecycle(request))
    assert response.status == "success"
    assert store.lifecycle == [request]
