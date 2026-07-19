from __future__ import annotations

import asyncio
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

from api.game.application import (
    AuthoritativeRenderer,
    ContentSnapshot,
    ContentSnapshotStore,
    DirectActionParser,
    GameService,
)
from api.game.config import GameConfigSnapshot, GameConfigStore
from api.game.engine import (
    CampaignEngine,
    ScriptedRandomSource,
    default_campaign_content,
    validate_persisted_state,
)
from api.game.models.api import GameActionRequest
from api.game.store import GameStore

NOW = datetime(2026, 5, 6, 7, 8, 9, tzinfo=timezone.utc)


class FailingAISpy:
    def __init__(self) -> None:
        self.calls = 0

    def factory(self):
        self.calls += 1
        raise AssertionError("direct mode must never initialize or call AI")


def _request(name: str, revision: int, **arguments: str | int | bool) -> GameActionRequest:
    request_id = uuid4()
    return GameActionRequest.model_validate({
        "request_id": str(request_id),
        "idempotency_key": str(request_id),
        "network_id": "libera",
        "identity": {"kind": "unregistered_nick", "value": "alice"},
        "display_nick": "Alice",
        "source": {"kind": "pm", "channel": "", "effective_prefix": "!"},
        "operation": "action",
        "mode": "direct",
        "expected_state_revision": revision,
        "action": {"name": name, "arguments": arguments},
        "client_context": {"content_policy_revision": 1, "configuration_revision": 1},
    })


def test_direct_mode_completes_campaign_without_ai_and_keeps_private_metadata(tmp_path: Path) -> None:
    config = GameConfigSnapshot(
        enabled=True,
        database_path="data/game.db",
        database_pool_size=2,
        database_busy_timeout_ms=2000,
        page_size=3,
        max_choices_per_page=3,
    )
    content = default_campaign_content("direct-v1")
    validator = lambda value, revision: validate_persisted_state(
        value, revision, config.campaign, content,
    )
    store = GameStore.open(
        config,
        repository_root=tmp_path,
        invariant_validator=validator,
        now_factory=lambda: NOW,
    )
    ai = FailingAISpy()
    service = GameService(
        configs=GameConfigStore(config),
        contents=ContentSnapshotStore(ContentSnapshot(
            version=content.version,
            records=(("campaign", content),),
        )),
        parser=DirectActionParser(),
        store=store,
        engine=CampaignEngine(),
        renderer=AuthoritativeRenderer(),
        ai_adapter_factory=ai.factory,
        random_source_factory=lambda: ScriptedRandomSource((0, 0, 0, 0)),
        now_factory=lambda: NOW,
    )
    responses = []

    def invoke(name: str, revision: int, **arguments: str | int | bool):
        response = asyncio.run(service.handle_action(_request(name, revision, **arguments)))
        responses.append(response)
        return response

    try:
        # Authored help works without creating a campaign and represents the
        # parser's bounded unknown/malformed fallback marker.
        fallback = invoke("help", 0, fallback=True)
        assert fallback.result_category == "unknown_action"
        assert not fallback.state_changed and fallback.state_revision == 0
        assert "not recognized" in fallback.deliveries[0].lines[0]
        assert store.load_state("libera", "unregistered_nick", "alice") is None

        started = invoke("start", 0)
        revision = started.state_revision
        assert revision == 1 and started.state_changed
        assert "Content notice: Standard fictionalized content" in started.deliveries[0].lines[0]
        assert any(item.input == "next" for item in started.continuations)

        first_context = started.menu_context
        first_tokens = {item.input for item in started.continuations if item.kind == "choice"}
        next_binding = next(item for item in started.continuations if item.input == "next")
        page = invoke("page", revision, page=next_binding.arguments.page)
        assert not page.state_changed and page.state_revision == revision
        assert page.menu_context is not None and first_context is not None
        assert page.menu_context.id != first_context.id
        page_tokens = {item.input for item in page.continuations if item.kind == "choice"}
        assert first_tokens.isdisjoint(page_tokens)
        assert store.load_state("libera", "unregistered_nick", "alice")["state_revision"] == revision

        status = invoke("status", revision)
        inventory = invoke("inventory", revision)
        help_response = invoke("help", revision)
        privacy = invoke("privacy", revision)
        assert "Status: active" in status.deliveries[0].lines[0]
        assert "Inventory: medkit x1" in inventory.deliveries[0].lines[0]
        assert "Stable commands:" in help_response.deliveries[0].lines[0]
        assert "does not store arbitrary PM conversation" in privacy.deliveries[0].lines[0]
        assert all(not response.state_changed for response in (status, inventory, help_response, privacy))

        # One deterministic direct-only campaign path.
        for name, arguments in (
            ("travel", {"destination_id": "docks"}),
            ("investigate", {}),
            ("travel", {"destination_id": "haven"}),
            ("travel", {"destination_id": "archive"}),
            ("investigate", {}),
            ("travel", {"destination_id": "haven"}),
            ("travel", {"destination_id": "spire"}),
            ("finalize", {}),
        ):
            result = invoke(name, revision, **arguments)
            assert result.status == "success" and result.state_changed
            revision = result.state_revision

        completed = store.load_state("libera", "unregistered_nick", "alice")
        assert completed["lifecycle"] == "completed"
        assert completed["state_revision"] == revision

        credits = invoke("credits", revision)
        credit_text = credits.deliveries[0].lines[0]
        assert "FSF Avenger" in credit_text
        assert "© 2026 Britney Lozza / CerberusGames.ca" in credit_text
        assert "TEMP-GAME.pas states GPLv3" in credit_text
        assert "MIT license does not relicense upstream material" in credit_text

        confirmation = invoke("reset", revision)
        token = next(item.input for item in confirmation.continuations if item.kind == "confirmation")
        assert not confirmation.state_changed and token.startswith("r-")
        before_bad_reset = store.load_state("libera", "unregistered_nick", "alice")
        bad_reset = invoke("reset", revision, token="r-aaaaaa")
        assert bad_reset.status == "error" and not bad_reset.state_changed
        assert store.load_state("libera", "unregistered_nick", "alice") == before_bad_reset

        reset = invoke("reset", revision, token=token)
        assert reset.status == "success" and reset.state_changed
        revision = reset.state_revision
        assert revision == before_bad_reset["state_revision"] + 1

        quit_response = invoke("quit", revision)
        assert quit_response.result_category == "campaign_quit"
        assert not quit_response.state_changed and quit_response.state_revision == revision

        assert ai.calls == 0
        assert all(
            response.deliveries and all(delivery.target == "pm" for delivery in response.deliveries)
            for response in responses
        )
    finally:
        store.close()
