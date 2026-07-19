from __future__ import annotations

import json
import sqlite3
from datetime import datetime, timedelta, timezone
from pathlib import Path
from uuid import uuid4

import pytest

from api.game.application import ContentSnapshot, EngineContext, GameServiceError, NormalizedAction, StoreActionRequest
from api.game.config import CampaignConfig, GameConfigSnapshot
from api.game.engine import CampaignEngine, ScriptedRandomSource, default_campaign_content, validate_persisted_state
from api.game.models.api import ErrorCategory
from api.game.store import GameStore

NOW = datetime(2026, 4, 5, 6, 7, 8, tzinfo=timezone.utc)


def lifecycle_config(*, countdown: int = 3) -> GameConfigSnapshot:
    return GameConfigSnapshot(
        enabled=True,
        database_path="data/game.db",
        database_pool_size=2,
        database_busy_timeout_ms=2000,
        reset_confirmation_ttl_seconds=60,
        campaign=CampaignConfig(starting_countdown=countdown),
    )


def open_lifecycle_store(tmp_path: Path, config: GameConfigSnapshot, clock: dict[str, datetime]):
    content = default_campaign_content("lifecycle-v1")
    validator = lambda value, revision: validate_persisted_state(
        value, revision, config.campaign, content,
    )
    store = GameStore.open(
        config,
        repository_root=tmp_path,
        invariant_validator=validator,
        now_factory=lambda: clock["now"],
    )
    return store, CampaignEngine(), content


def execute(
    store: GameStore,
    engine: CampaignEngine,
    content,
    config: GameConfigSnapshot,
    clock: dict[str, datetime],
    *,
    identity: str,
    revision: int,
    name: str,
    token: str | None = None,
):
    arguments = (("token", token),) if token is not None else ()
    action = NormalizedAction(name=name, arguments=arguments)
    request = StoreActionRequest(
        request_id=uuid4(),
        idempotency_key=uuid4(),
        network_id="libera",
        identity_kind="unregistered_nick",
        identity_value=identity,
        expected_state_revision=revision,
        action=action,
        configuration_revision=1,
        content_policy_revision=1,
        display_nick=identity.title(),
        engine_version=engine.version,
        content_version=content.version,
        state_schema_version=1,
    )
    context = EngineContext(
        config=config,
        content=ContentSnapshot(version=content.version, records=(("campaign", content),)),
        random=ScriptedRandomSource(()),
        now=clock["now"],
        identity=("libera", "unregistered_nick", identity),
    )
    return request, store.execute(
        request,
        lambda state: engine.transition(state, action, context),
    )


def test_start_creates_once_then_resumes_and_quit_only_clears_context(tmp_path: Path) -> None:
    config = lifecycle_config()
    clock = {"now": NOW}
    store, engine, content = open_lifecycle_store(tmp_path, config, clock)
    try:
        _, started = execute(
            store, engine, content, config, clock,
            identity="alice", revision=0, name="start",
        )
        assert started.state_changed and started.state_revision == 1

        _, advanced = execute(
            store, engine, content, config, clock,
            identity="alice", revision=1, name="advance",
        )
        committed = store.load_state("libera", "unregistered_nick", "alice")
        assert advanced.state_revision == 2
        assert committed["day"] == 2

        _, resumed = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="start",
        )
        assert resumed.result_category == "campaign_resumed"
        assert not resumed.state_changed and resumed.state_revision == 2
        assert store.load_state("libera", "unregistered_nick", "alice") == committed

        _, quit_result = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="quit",
        )
        assert quit_result.result_category == "campaign_quit"
        assert not quit_result.state_changed and quit_result.choices == ()
        assert store.load_state("libera", "unregistered_nick", "alice") == committed
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            assert connection.execute(
                "SELECT count(*) FROM menu_contexts WHERE superseded_at IS NULL"
            ).fetchone()[0] == 0

        _, after_quit = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="start",
        )
        assert not after_quit.state_changed
        assert after_quit.state_revision == 2
        assert after_quit.choices
        assert store.load_state("libera", "unregistered_nick", "alice") == committed
    finally:
        store.close()


def test_valid_reset_of_failed_campaign_is_hashed_archived_audited_and_monotonic(tmp_path: Path) -> None:
    config = lifecycle_config(countdown=1)
    clock = {"now": NOW}
    store, engine, content = open_lifecycle_store(tmp_path, config, clock)
    try:
        execute(store, engine, content, config, clock, identity="alice", revision=0, name="start")
        _, failed = execute(
            store, engine, content, config, clock,
            identity="alice", revision=1, name="advance",
        )
        assert failed.next_state.lifecycle.value == "failed"

        _, replay_gate = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="start",
        )
        assert not replay_gate.state_changed
        assert store.load_state("libera", "unregistered_nick", "alice")["lifecycle"] == "failed"

        _, confirmation = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="reset",
        )
        token = confirmation.choices[0].input
        assert confirmation.result_category == "reset_confirmation_required"
        assert token.startswith("r-") and len(token) == 28
        before_reset = store.load_state("libera", "unregistered_nick", "alice")
        assert before_reset["state_revision"] == 2 and before_reset["lifecycle"] == "failed"

        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            row = connection.execute(
                "SELECT token_hash, identity_key_hash, issued_revision, used_at FROM reset_tokens"
            ).fetchone()
            assert len(row[0]) == 32 and len(row[1]) == 32
            assert row[0] != token.encode("ascii")
            assert row[2:] == (2, None)
            assert token not in "\n".join(connection.iterdump())

        reset_request, reset_result = execute(
            store, engine, content, config, clock,
            identity="alice", revision=2, name="reset", token=token,
        )
        state = store.load_state("libera", "unregistered_nick", "alice")
        assert reset_result.result_category == "campaign_reset"
        assert reset_result.state_changed and reset_result.state_revision == 3
        assert state["state_revision"] == 3
        assert state["lifecycle"] == "active"
        assert state["location_id"] == config.campaign.starting_location
        assert state["day"] == config.campaign.starting_day
        assert state["countdown_remaining"] == config.campaign.starting_countdown
        assert state["health"] == config.campaign.starting_health
        assert state["currency"] == config.campaign.starting_currency
        assert dict(state["inventory"]) == config.campaign.inventory_map()

        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            assert connection.execute(
                "SELECT used_at IS NOT NULL FROM reset_tokens"
            ).fetchone()[0] == 1
            archive = connection.execute(
                "SELECT prior_revision, state_ciphertext_or_json, reason FROM session_archives"
            ).fetchone()
            assert archive[0] == 2 and json.loads(archive[1])["lifecycle"] == "failed"
            assert archive[2] == "reset"
            audit = connection.execute(
                """SELECT request_id, pre_revision, post_revision, result_category
                   FROM game_audits WHERE event_type = 'reset'"""
            ).fetchone()
            assert audit == (str(reset_request.request_id), 2, 3, "campaign_reset")
            assert connection.execute(
                "SELECT count(*) FROM action_results WHERE result_json LIKE ?",
                (f"%{token}%",),
            ).fetchone()[0] == 0

        with pytest.raises(GameServiceError) as reused:
            execute(
                store, engine, content, config, clock,
                identity="alice", revision=3, name="reset", token=token,
            )
        assert reused.value.category == ErrorCategory.STALE_CONTEXT
        assert store.load_state("libera", "unregistered_nick", "alice") == state
    finally:
        store.close()


def test_expired_cross_identity_and_stale_reset_tokens_preserve_state(tmp_path: Path) -> None:
    config = lifecycle_config()
    clock = {"now": NOW}
    store, engine, content = open_lifecycle_store(tmp_path, config, clock)
    try:
        for identity in ("alice", "bob"):
            execute(store, engine, content, config, clock, identity=identity, revision=0, name="start")

        _, alice_confirmation = execute(
            store, engine, content, config, clock,
            identity="alice", revision=1, name="reset",
        )
        alice_token = alice_confirmation.choices[0].input
        bob_before = store.load_state("libera", "unregistered_nick", "bob")
        with pytest.raises(GameServiceError) as cross_identity:
            execute(
                store, engine, content, config, clock,
                identity="bob", revision=1, name="reset", token=alice_token,
            )
        assert cross_identity.value.category == ErrorCategory.STALE_CONTEXT
        assert store.load_state("libera", "unregistered_nick", "bob") == bob_before

        _, bob_confirmation = execute(
            store, engine, content, config, clock,
            identity="bob", revision=1, name="reset",
        )
        bob_token = bob_confirmation.choices[0].input
        clock["now"] += timedelta(seconds=config.reset_confirmation_ttl_seconds + 1)
        with pytest.raises(GameServiceError) as expired:
            execute(
                store, engine, content, config, clock,
                identity="bob", revision=1, name="reset", token=bob_token,
            )
        assert expired.value.category == ErrorCategory.STALE_CONTEXT
        assert store.load_state("libera", "unregistered_nick", "bob") == bob_before

        _, current_confirmation = execute(
            store, engine, content, config, clock,
            identity="alice", revision=1, name="reset",
        )
        stale_token = current_confirmation.choices[0].input
        execute(
            store, engine, content, config, clock,
            identity="alice", revision=1, name="advance",
        )
        alice_after_advance = store.load_state("libera", "unregistered_nick", "alice")
        with pytest.raises(GameServiceError) as stale:
            execute(
                store, engine, content, config, clock,
                identity="alice", revision=2, name="reset", token=stale_token,
            )
        assert stale.value.category == ErrorCategory.STALE_CONTEXT
        assert store.load_state("libera", "unregistered_nick", "alice") == alice_after_advance
    finally:
        store.close()
