from __future__ import annotations

import inspect
import json
from pathlib import Path
from uuid import uuid4

from fastapi.testclient import TestClient

import api.main as api_main
from api.game.models.api import (
    GameActionRequest, GameActionResponse, LifecycleRequest, LifecycleResponse,
)
from api.game.runtime import GameRuntime, GameStartupResult, game_runtime
from api.game.store import MigrationError
from api.main import app

from .test_config_contracts import valid_request


class IsolatedGameService:
    def __init__(self) -> None:
        self.actions = 0
        self.lifecycle = 0

    async def handle_action(self, request: GameActionRequest) -> GameActionResponse:
        self.actions += 1
        return GameActionResponse(
            request_id=request.request_id,
            status="success",
            result_category="menu",
            state_revision=request.expected_state_revision,
            state_changed=False,
            deliveries=[{"target": "pm", "lines": ["Game menu"]}],
        )

    async def handle_lifecycle(self, request: LifecycleRequest) -> LifecycleResponse:
        self.lifecycle += 1
        return LifecycleResponse(request_id=request.request_id, status="success")


def test_game_endpoints_are_ready_validated_and_isolated(
    tmp_path: Path, monkeypatch,
) -> None:
    config_path = tmp_path / "game.toml"
    config_path.write_text("[game]\nenabled = true\n", encoding="utf-8")
    service = IsolatedGameService()
    game_runtime.start(
        config_path,
        initializer=lambda config: GameStartupResult(
            service=service,
            schema_version=1,
            engine_version="test-engine",
            content_version="test-content",
        ),
    )

    def forbidden_loader():
        raise AssertionError("generic command loading must remain untouched")

    monkeypatch.setattr(api_main, "get_command_loader", forbidden_loader)
    client = TestClient(app)
    request = valid_request()
    response = client.post(
        "/game/action", json=request,
        headers={"X-Request-ID": request["request_id"]},
    )
    assert response.status_code == 200
    assert response.json()["request_id"] == request["request_id"]
    assert response.json()["deliveries"] == [{"target": "pm", "lines": ["Game menu"]}]
    assert service.actions == 1

    lifecycle_id = str(uuid4())
    lifecycle = {
        "request_id": lifecycle_id,
        "network_id": "libera",
        "operation": "invalidate_context",
        "identity": {"kind": "unregistered_nick", "value": "alice"},
        "configuration_revision": 1,
    }
    lifecycle_response = client.post(
        "/game/lifecycle", json=lifecycle,
        headers={"X-Request-ID": lifecycle_id},
    )
    assert lifecycle_response.status_code == 200
    assert lifecycle_response.json() == {"request_id": lifecycle_id, "status": "success", "error": None}
    assert service.lifecycle == 1

    health_id = str(uuid4())
    health = client.get("/game/health", headers={"X-Request-ID": health_id})
    assert health.status_code == 200
    assert health.json()["status"] == "ready"
    bad_health = client.get("/game/health", headers={"X-Request-ID": "not-a-uuid"})
    assert bad_health.status_code == 400

    # The isolated modules have no executable imports or references to generic
    # command loading, mention handling, chat history, streaming, or tools.
    import api.game.router as game_router_module
    source = inspect.getsource(game_router_module)
    forbidden = ("api.loader", "api.mention", "api.tools", "chat_history", "get_command_loader")
    assert not any(name in source for name in forbidden)

    game_runtime.install_service(None)
    game_runtime.start()


def test_disabled_game_is_diagnostic_and_rejects_actions_without_generic_dispatch(
    tmp_path: Path, monkeypatch,
) -> None:
    config_path = tmp_path / "disabled-game.toml"
    config_path.write_text("[game]\nenabled = false\n", encoding="utf-8")
    game_runtime.start(config_path)
    calls = {"loader": 0}

    def forbidden_loader():
        calls["loader"] += 1
        raise AssertionError("game requests must not load generic commands")

    monkeypatch.setattr(api_main, "get_command_loader", forbidden_loader)
    client = TestClient(app)
    request = valid_request()
    response = client.post(
        "/game/action",
        json=request,
        headers={"X-Request-ID": request["request_id"]},
    )
    assert response.status_code == 200
    assert response.json()["status"] == "unavailable"
    assert response.json()["error"]["category"] == "game_unavailable"
    health = client.get("/game/health").json()
    assert health["status"] == "disabled"
    assert set(health) == {
        "status", "database_available", "schema_version", "migration_status",
        "engine_version", "content_version", "config_revision", "ai_status",
        "error_category",
    }
    assert calls == {"loader": 0}


def test_malformed_game_request_does_not_invoke_installed_service() -> None:
    service = IsolatedGameService()
    game_runtime.install_service(service)
    client = TestClient(app)
    request = valid_request()
    request["identity"]["value"] = "bad\nidentity"
    response = client.post(
        "/game/action",
        json=request,
        headers={"X-Request-ID": request["request_id"]},
    )
    assert response.status_code == 422
    assert service.actions == 0
    assert service.lifecycle == 0
    game_runtime.start()


def test_concrete_migration_failure_degrades_only_game_readiness(tmp_path: Path) -> None:
    config_path = tmp_path / "game.toml"
    config_path.write_text("[game]\nenabled = true\n", encoding="utf-8")
    runtime = GameRuntime()

    def failed_initializer(config):
        del config
        raise MigrationError("migration 002 failed", version=2, name="broken")

    runtime.start(config_path, initializer=failed_initializer)
    health = runtime.health()
    assert health.status == "degraded"
    assert health.database_available
    assert health.schema_version == 2
    assert health.migration_status == "failed"
    assert health.error_category.value == "migration_failed"
    assert not runtime.ready()


def test_generic_command_routes_reject_reserved_game_namespace_before_loader(
    monkeypatch,
) -> None:
    service = IsolatedGameService()
    game_runtime.install_service(service)
    loader_calls = 0

    class ForbiddenLoader:
        def get_command(self, command):
            del command
            raise AssertionError("reserved game command reached generic loader")

        def get_metadata(self, command):
            del command
            raise AssertionError("reserved game metadata reached generic loader")

    def loader():
        nonlocal loader_calls
        loader_calls += 1
        return ForbiddenLoader()

    monkeypatch.setattr(api_main, "get_command_loader", loader)
    client = TestClient(app)
    request_id = str(uuid4())
    payload = {
        "request_id": request_id,
        "command": "AvEnGeR",
        "args": ["start"],
        "nick": "alice",
        "hostmask": None,
        "network": "libera",
        "channel": "#test",
        "is_pm": False,
    }

    response = client.post("/command", json=payload)
    assert response.status_code == 200
    assert response.json() == {
        "request_id": request_id,
        "status": "error",
        "message": "Game commands require the dedicated /game/action route",
        "required_level": None,
        "streaming": False,
    }

    stream_response = client.post("/command/stream", json=payload)
    assert stream_response.status_code == 200
    chunks = [json.loads(line) for line in stream_response.text.splitlines()]
    assert chunks == [{
        "request_id": request_id,
        "status": "error",
        "message": "Game commands require the dedicated /game/action route",
        "streaming": False,
    }]
    assert loader_calls == 0
    assert service.actions == 0
    assert service.lifecycle == 0
    game_runtime.start()


def test_feature_disable_rollback_retains_game_database(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.chdir(tmp_path)
    data_dir = tmp_path / "data"
    data_dir.mkdir()
    database_path = data_dir / "game.db"
    retained = b"existing committed game data"
    database_path.write_bytes(retained)
    config_path = tmp_path / "game.toml"
    config_path.write_text(
        "[game]\nenabled = false\ndatabase_path = \"data/game.db\"\n",
        encoding="utf-8",
    )
    runtime = GameRuntime()

    def forbidden_initializer(config):
        del config
        raise AssertionError("disabled rollback initialized game persistence")

    runtime.start(config_path, initializer=forbidden_initializer)
    assert runtime.health().status == "disabled"
    assert not runtime.ready()
    assert database_path.read_bytes() == retained
