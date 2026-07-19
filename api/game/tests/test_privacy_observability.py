from __future__ import annotations

import asyncio
import io
import json
import logging
import sqlite3
from datetime import datetime, timedelta, timezone
from pathlib import Path

from api.game.application import (
    AuthoritativeRenderer,
    AuthoritativeResult,
    ContentSnapshot,
    ContentSnapshotStore,
    DirectActionParser,
    GameService,
)
from api.game.config import GameConfigSnapshot, GameConfigStore
from api.game.models.api import GameActionRequest, GameHealthResponse
from api.game.observability import GameEvent, GameTelemetry, action_event
from api.game.store import GameStore

from .test_application import RecordingEngine, TransactionalStore
from .test_config_contracts import valid_request
from .test_store_sqlite import create_session, request, result

NOW = datetime(2026, 6, 1, 12, 0, tzinfo=timezone.utc)


class RecordingObserver:
    def __init__(self) -> None:
        self.events: list[GameEvent] = []

    def observe(self, event: GameEvent) -> None:
        self.events.append(event)


def store_config(**updates) -> GameConfigSnapshot:
    values = {
        "enabled": True,
        "database_path": "data/game.db",
        "database_pool_size": 2,
        "database_busy_timeout_ms": 1000,
        "maintenance_batch_size": 10,
    }
    values.update(updates)
    return GameConfigSnapshot(**values)


def test_structured_events_are_allowlisted_and_request_id_is_continuous() -> None:
    observer = RecordingObserver()
    service = GameService(
        configs=GameConfigStore(GameConfigSnapshot(enabled=True)),
        contents=ContentSnapshotStore(ContentSnapshot(version="test")),
        parser=DirectActionParser(),
        store=TransactionalStore(),
        engine=RecordingEngine(),
        renderer=AuthoritativeRenderer(),
        observer=observer,
    )
    request_model = GameActionRequest.model_validate(valid_request())
    response = asyncio.run(service.handle_action(request_model))

    assert response.request_id == request_model.request_id
    assert len(observer.events) == 1
    event = observer.events[0]
    assert event.request_id == str(request_model.request_id)
    assert event.pre_revision == 0 and event.post_revision == 1
    assert event.result_category == "campaign_started"
    assert event.session_ref.startswith("session-")
    assert request_model.identity.value not in json.dumps(event.as_log_record())
    assert set(event.as_log_record()) == {
        "event", "request_id", "network", "session_ref", "action_type",
        "pre_revision", "post_revision", "latency_ms", "result_category",
        "error_category", "configuration_revision", "content_policy_revision",
    }


def test_routine_logs_and_metrics_have_no_forbidden_fields_or_values() -> None:
    output = io.StringIO()
    logger = logging.getLogger("test.game.telemetry")
    logger.handlers = [logging.StreamHandler(output)]
    logger.propagate = False
    logger.setLevel(logging.INFO)
    telemetry = GameTelemetry(logger)
    event = action_event(
        request_id=GameActionRequest.model_validate(valid_request()).request_id,
        network_id="libera",
        identity_kind="unregistered_nick",
        identity_value="alice",
        action_type="status",
        pre_revision=3,
        post_revision=3,
        latency_ms=4,
        result_category="status",
        error_category=None,
        configuration_revision=1,
        content_policy_revision=1,
    )
    telemetry.observe(event)
    serialized = output.getvalue().lower()
    for forbidden in (
        "alice", "hostmask", "password", "token", "raw_prompt", "raw_ai_prompt",
        "pm_text", "narration", "choice_token", "reset_token",
    ):
        assert forbidden not in serialized
    metrics = telemetry.metrics_snapshot()
    assert metrics["game_actions_total"] == {"status|status|none": 1}


def test_health_projection_contains_only_operator_safe_fields() -> None:
    health = GameHealthResponse(
        status="ready",
        database_available=True,
        schema_version=3,
        migration_status="current",
        engine_version="1",
        content_version="standard-v1",
        config_revision=1,
        ai_status="disabled",
    ).model_dump(mode="json")
    assert set(health) == {
        "status", "database_available", "schema_version", "migration_status",
        "engine_version", "content_version", "config_revision", "ai_status",
        "error_category",
    }
    assert not ({"database_path", "identity", "player_count", "prompt", "hostmask", "token"} & set(health))


def test_authenticated_deletion_cascades_session_children(tmp_path: Path) -> None:
    store = GameStore.open(store_config(), repository_root=tmp_path, now_factory=lambda: NOW)
    try:
        create_session(store, "alice")
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            session_id = connection.execute(
                "SELECT session_id FROM game_sessions WHERE identity_value = 'alice'"
            ).fetchone()[0]
            connection.execute(
                "INSERT INTO session_archives(archive_id, session_id, prior_revision, state_ciphertext_or_json, reason, expires_at, created_at) "
                "VALUES ('archive-1', ?, 1, '{}', 'test', ?, ?)",
                (session_id, (NOW + timedelta(days=1)).isoformat(), NOW.isoformat()),
            )
            connection.commit()

        assert store.delete_authenticated_session("libera", "unregistered_nick", "alice")
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            for table in (
                "game_sessions", "content_preferences", "action_results", "menu_contexts",
                "menu_choices", "milestones", "reset_tokens", "game_audits", "session_archives",
            ):
                assert connection.execute(f"SELECT count(*) FROM {table}").fetchone()[0] == 0
    finally:
        store.close()


def test_retention_cleanup_is_bounded_keeps_other_sessions_and_aggregates(tmp_path: Path) -> None:
    clock = {"now": NOW}
    store = GameStore.open(store_config(), repository_root=tmp_path, now_factory=lambda: clock["now"])
    try:
        create_session(store, "alice")
        create_session(store, "bob")
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            connection.execute(
                "UPDATE game_sessions SET expires_at = ? WHERE identity_value = 'alice'",
                ((NOW - timedelta(seconds=1)).isoformat(),),
            )
            connection.commit()
        cleanup = store.run_maintenance(NOW)
        assert cleanup.status == "completed" and cleanup.deleted_sessions == 1
        assert store.load_state("libera", "unregistered_nick", "alice") is None
        assert store.load_state("libera", "unregistered_nick", "bob") is not None
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            assert connection.execute(
                "SELECT value FROM aggregate_metrics WHERE metric_name = 'expired_sessions_deleted'"
            ).fetchone()[0] == 1
            assert connection.execute(
                "SELECT status FROM maintenance_runs WHERE run_id = ?", (cleanup.run_id,)
            ).fetchone()[0] == "completed"
    finally:
        store.close()


def test_cleanup_failure_rolls_back_and_status_warns_before_renewal(tmp_path: Path) -> None:
    clock = {"now": NOW}
    config = store_config()
    store = GameStore.open(config, repository_root=tmp_path, now_factory=lambda: clock["now"])
    try:
        create_session(store, "alice")
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            connection.execute(
                "UPDATE game_sessions SET expires_at = ? WHERE identity_value = 'alice'",
                ((NOW + timedelta(days=2)).isoformat(),),
            )
            connection.commit()

        status_request = request(revision=1, action="status")
        status_result = store.execute(
            status_request,
            lambda state: AuthoritativeResult(
                result_category="status",
                state_revision=1,
                state_changed=False,
                facts=("Status is active.",),
            ),
        )
        assert status_result.facts[0].startswith("Inactive-save notice:")
        with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
            renewed = datetime.fromisoformat(connection.execute(
                "SELECT expires_at FROM game_sessions WHERE identity_value = 'alice'"
            ).fetchone()[0])
            assert renewed == NOW + timedelta(days=config.save_retention_days)
            connection.execute(
                "UPDATE game_sessions SET expires_at = ? WHERE identity_value = 'alice'",
                ((NOW - timedelta(seconds=1)).isoformat(),),
            )
            connection.execute(
                "CREATE TRIGGER prevent_test_delete BEFORE DELETE ON game_sessions "
                "BEGIN SELECT RAISE(ABORT, 'test cleanup failure'); END"
            )
            connection.commit()

        failed = store.run_maintenance(NOW)
        assert failed.status == "failed"
        assert store.load_state("libera", "unregistered_nick", "alice") is not None
    finally:
        store.close()
