from __future__ import annotations

from pathlib import Path
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from pydantic import ValidationError

from api.game.config import (
    BoundaryLimits,
    CampaignConfig,
    GameConfigSnapshot,
    StartingInventoryEntry,
    load_game_config,
)
from api.game.models.api import GameActionRequest
from api.game.runtime import (
    ContentStartupFailure,
    DatabaseStartupFailure,
    GameRuntime,
    GameStartupResult,
    MigrationStartupFailure,
    game_runtime,
)
from api.main import app


def valid_request() -> dict:
    request_id = str(uuid4())
    return {
        "request_id": request_id,
        "idempotency_key": request_id,
        "network_id": "libera",
        "identity": {"kind": "unregistered_nick", "value": "alice"},
        "display_nick": "Alice",
        "source": {"kind": "pm", "channel": "", "effective_prefix": "!"},
        "operation": "action",
        "mode": "direct",
        "expected_state_revision": 0,
        "action": {"name": "start", "arguments": {}},
        "client_context": {"content_policy_revision": 1, "configuration_revision": 1},
    }


def test_default_snapshot_is_safe_and_local() -> None:
    config = GameConfigSnapshot()
    assert not config.enabled
    assert not config.channel_play_enabled
    assert not config.milestone_announcements_enabled
    assert not config.adult_content_enabled
    assert not config.real_person_content_enabled
    assert not config.ai_enhancement_enabled
    assert config.database_path == "data/game.db"
    assert config.standard_content_profile == "standard"


def test_shipped_snapshot_has_no_secret_fields() -> None:
    config_path = Path("api/config/game_settings.toml")
    raw = config_path.read_text(encoding="utf-8").lower()
    assert "api_key =" not in raw
    assert "password =" not in raw
    assert "secret =" not in raw
    loaded = load_game_config(config_path)
    assert loaded == GameConfigSnapshot(enabled=True)
    assert loaded.pm_enabled
    assert not loaded.channel_play_enabled
    assert not loaded.milestone_announcements_enabled
    assert not loaded.adult_content_enabled
    assert not loaded.real_person_content_enabled
    assert not loaded.ai_enhancement_enabled
    assert loaded.channel_allowlist == ()
    assert loaded.milestones.destinations == ()
    assert loaded.campaign.starting_inventory == (
        StartingInventoryEntry(item_id="medkit", quantity=1),
    )
    assert loaded.campaign.inventory_map() == {"medkit": 1}


def test_starting_inventory_is_typed_ordered_and_legacy_pairs_remain_compatible() -> None:
    structured = CampaignConfig(starting_inventory=[
        {"item_id": "medkit", "quantity": 2},
        {"item_id": "signal_key", "quantity": 1},
    ])
    assert all(isinstance(entry, StartingInventoryEntry) for entry in structured.starting_inventory)
    assert tuple(entry.item_id for entry in structured.starting_inventory) == ("medkit", "signal_key")
    assert structured.inventory_map() == {"medkit": 2, "signal_key": 1}

    legacy = CampaignConfig(starting_inventory=[["medkit", 1]])
    assert legacy.starting_inventory == (StartingInventoryEntry(item_id="medkit", quantity=1),)


@pytest.mark.parametrize(("inventory", "limits"), [
    ([{"item_id": "", "quantity": 1}], {}),
    ([{"item_id": "   ", "quantity": 1}], {}),
    ([{"item_id": "medkit", "quantity": 0}], {}),
    ([{"item_id": "medkit", "quantity": 3}], {"maximum_item_quantity": 2}),
    (
        [{"item_id": "medkit", "quantity": 1}, {"item_id": "medkit", "quantity": 1}],
        {},
    ),
    (
        [{"item_id": "medkit", "quantity": 2}, {"item_id": "signal_key", "quantity": 2}],
        {"inventory_capacity": 3},
    ),
])
def test_starting_inventory_validation_is_preserved(
    inventory: list[dict[str, object]], limits: dict[str, int],
) -> None:
    with pytest.raises(ValidationError):
        CampaignConfig(starting_inventory=inventory, **limits)


def test_runtime_rejects_unknown_starting_inventory_items() -> None:
    config = GameConfigSnapshot(
        enabled=True,
        campaign=CampaignConfig(starting_inventory=[
            {"item_id": "unknown_item", "quantity": 1},
        ]),
    )
    with pytest.raises(ContentStartupFailure):
        GameRuntime._default_components(config)


@pytest.mark.parametrize("field,value", [
    ("database_path", "../game.db"),
    ("database_path", "file:///tmp/game.db"),
    ("backup_directory", "/tmp/backups"),
    ("max_input_bytes", 4097),
    ("page_size", 13),
    ("config_revision", 0),
])
def test_malformed_or_unsafe_configuration_is_rejected(field: str, value: object) -> None:
    candidate = GameConfigSnapshot().model_dump()
    candidate[field] = value
    with pytest.raises(ValidationError):
        GameConfigSnapshot.model_validate(candidate)


def test_boundary_reconciliation_uses_stricter_values() -> None:
    left = BoundaryLimits(max_input_bytes=512, max_menu_lines=4, max_choices_per_page=6, max_narration_bytes=600, action_timeout_seconds=10)
    right = BoundaryLimits(max_input_bytes=256, max_menu_lines=8, max_choices_per_page=3, max_narration_bytes=800, action_timeout_seconds=5)
    assert left.stricter(right) == BoundaryLimits(max_input_bytes=256, max_menu_lines=4, max_choices_per_page=3, max_narration_bytes=600, action_timeout_seconds=5)


def test_contract_rejects_malformed_oversized_and_action_specific_fields() -> None:
    request = valid_request()
    assert GameActionRequest.model_validate(request)

    malformed = valid_request()
    malformed["identity"]["value"] = "bad\nidentity"
    with pytest.raises(ValidationError):
        GameActionRequest.model_validate(malformed)

    oversized = valid_request()
    oversized["display_nick"] = "x" * 65
    with pytest.raises(ValidationError):
        GameActionRequest.model_validate(oversized)

    attack = valid_request()
    attack["action"]["name"] = "attack"
    with pytest.raises(ValidationError):
        GameActionRequest.model_validate(attack)

    generic = valid_request()
    generic["action"]["arguments"]["arbitrary"] = {"nested": "data"}
    with pytest.raises(ValidationError):
        GameActionRequest.model_validate(generic)


def test_request_id_header_must_match_and_input_is_bounded() -> None:
    game_runtime.start()
    client = TestClient(app)
    request = valid_request()
    mismatch = client.post("/game/action", json=request, headers={"X-Request-ID": str(uuid4())})
    assert mismatch.status_code == 400
    assert mismatch.json()["detail"]["category"] == "request_id_mismatch"

    request["mode"] = "ai_interpret"
    request["action"] = {"name": "ask", "arguments": {"text": "x" * 513}}
    oversized = client.post("/game/action", json=request, headers={"X-Request-ID": request["request_id"]})
    assert oversized.status_code == 400
    assert oversized.json()["detail"]["field"] == "action.arguments.text"


def test_game_startup_failure_degrades_only_game(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    config_path = tmp_path / "game.toml"
    config_path.write_text("[game]\nenabled = true\ndatabase_path = '../unsafe.db'\n", encoding="utf-8")
    runtime = GameRuntime()
    runtime.start(config_path)
    assert runtime.health().status == "degraded"
    assert runtime.health().error_category == "game_unavailable"

    # The application's unrelated health route remains registered and healthy;
    # game degradation does not remove command, mention, or callback surfaces.
    game_runtime.start(config_path)
    client = TestClient(app)
    assert client.get("/health").status_code == 200
    assert client.get("/health").json()["status"] == "ok"
    assert client.get("/game/health").json()["status"] == "degraded"
    paths = set(client.get("/openapi.json").json()["paths"])
    assert {"/command", "/mention", "/game/action", "/game/lifecycle"} <= paths


def test_enabled_game_does_not_require_openai_key(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    config_path = tmp_path / "game.toml"
    # Optional AI credentials may be absent while direct game startup remains ready.
    config_path.write_text(
        "[game]\nenabled = true\nai_enhancement_enabled = true\n",
        encoding="utf-8",
    )
    runtime = GameRuntime()
    runtime.start(
        config_path,
        initializer=lambda config: GameStartupResult(
            service=object(),  # Health-only startup seam for this focused test.
            schema_version=1,
            engine_version="test-engine",
            content_version="test-content",
        ),
    )
    assert runtime.health().status == "ready"
    assert runtime.health().ai_status == "disabled_missing_credentials"


def test_database_and_migration_failures_have_redacted_game_only_health(tmp_path: Path) -> None:
    config_path = tmp_path / "game.toml"
    config_path.write_text("[game]\nenabled = true\n", encoding="utf-8")

    for failure, category, database_available, migration_status in (
        (DatabaseStartupFailure(), "database_unavailable", False, "not_started"),
        (MigrationStartupFailure(schema_version=2), "migration_failed", True, "failed"),
    ):
        runtime = GameRuntime()

        def fail(config, error=failure):
            raise error

        runtime.start(config_path, initializer=fail)
        health = runtime.health().model_dump(mode="json")
        assert health["status"] == "degraded"
        assert health["error_category"] == category
        assert health["database_available"] is database_available
        assert health["migration_status"] == migration_status
        assert "database_path" not in health
        assert "error" not in health
