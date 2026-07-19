from __future__ import annotations

import os
import sqlite3
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from pathlib import Path
from uuid import uuid4

import pytest

from api.game.application import (
    AuthoritativeChoice,
    AuthoritativeResult,
    GameServiceError,
    NormalizedAction,
    StoreActionRequest,
)
from api.game.config import GameConfigSnapshot
from api.game.models.api import ErrorCategory
from api.game.store import (
    GameStore, MigrationError, MigrationRunner, SQLiteConnectionPool, UnsafeDatabasePath,
)


def config() -> GameConfigSnapshot:
    return GameConfigSnapshot(
        enabled=True,
        database_path="data/game.db",
        database_pool_size=4,
        database_busy_timeout_ms=2000,
    )


def request(
    *,
    revision: int,
    action: str = "start",
    identity: str = "alice",
    request_id=None,
    idempotency_key=None,
    arguments: tuple[tuple[str, object], ...] = (),
) -> StoreActionRequest:
    return StoreActionRequest(
        request_id=request_id or uuid4(),
        idempotency_key=idempotency_key or uuid4(),
        network_id="libera",
        identity_kind="unregistered_nick",
        identity_value=identity,
        expected_state_revision=revision,
        action=NormalizedAction(name=action, arguments=arguments),
        configuration_revision=1,
        content_policy_revision=1,
        display_nick=identity.title(),
        engine_version="test-engine",
        content_version="test-content",
        state_schema_version=1,
    )


def result(
    revision: int,
    state: dict,
    *,
    category: str = "changed",
    choices: bool = False,
    milestone: str | None = None,
) -> AuthoritativeResult:
    offered = ()
    context_id = None
    expires_at = None
    if choices:
        offered = (
            AuthoritativeChoice(input="look", kind="action", action="look"),
            AuthoritativeChoice(
                input="c-abcdef", kind="choice", action="travel",
                arguments=(("destination_id", "harbor"),), choice_token="c-abcdef",
            ),
        )
        context_id = f"m-{uuid4().hex}"
        expires_at = datetime.now(timezone.utc) + timedelta(minutes=15)
    return AuthoritativeResult(
        result_category=category,
        state_revision=revision,
        state_changed=True,
        facts=(f"Revision {revision} committed.",),
        choices=offered,
        menu_context_id=context_id,
        menu_expires_at=expires_at,
        milestones=(milestone,) if milestone else (),
        next_state=state,
        random_metadata=(("roll", revision),),
    )


def create_session(store: GameStore, identity: str = "alice", currency: int = 10) -> StoreActionRequest:
    first = request(revision=0, identity=identity)
    store.execute(
        first,
        lambda _: result(1, {
            "state_revision": 1,
            "lifecycle": "active",
            "currency": currency,
            "claimed_rewards": [],
        }, choices=True),
    )
    return first


def test_empty_and_current_migrations_and_secure_open_settings(tmp_path: Path) -> None:
    pool = SQLiteConnectionPool(config(), repository_root=tmp_path)
    runner = MigrationRunner(pool)
    first = runner.migrate()
    second = runner.migrate()

    assert first == second
    assert first.version == first.latest_available == 3
    assert not first.dirty
    with pool.connection() as connection:
        assert connection.execute("PRAGMA journal_mode").fetchone()[0].lower() == "wal"
        assert connection.execute("PRAGMA foreign_keys").fetchone()[0] == 1
        assert connection.execute("PRAGMA busy_timeout").fetchone()[0] == 2000
        assert connection.execute("PRAGMA synchronous").fetchone()[0] == 2
        assert [tuple(row) for row in connection.execute(
            "SELECT version, dirty FROM schema_migrations ORDER BY version"
        ).fetchall()] == [(1, 0), (2, 0), (3, 0)]
    assert pool.path.name == "game.db"
    assert pool.path != tmp_path / "data" / "bot.db"
    assert os.stat(pool.path).st_mode & 0o777 == 0o600
    pool.close()


def test_complete_atomic_write_and_validation_rollback(tmp_path: Path) -> None:
    store = GameStore.open(config(), repository_root=tmp_path)
    first = request(revision=0)
    committed = store.execute(
        first,
        lambda _: result(
            1,
            {"state_revision": 1, "lifecycle": "active", "currency": 12},
            choices=True,
            milestone="campaign_started",
        ),
    )
    assert committed.state_revision == 1

    with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
        assert connection.execute("SELECT count(*) FROM game_sessions").fetchone()[0] == 1
        assert connection.execute("SELECT count(*) FROM action_results").fetchone()[0] == 1
        assert connection.execute("SELECT count(*) FROM game_audits").fetchone()[0] == 1
        assert connection.execute("SELECT count(*) FROM menu_choices").fetchone()[0] == 2
        assert connection.execute("SELECT count(*) FROM milestones").fetchone()[0] == 1

    bad = request(revision=1, action="advance")
    with pytest.raises(GameServiceError) as invalid:
        store.execute(
            bad,
            lambda state: result(
                2,
                {**state, "state_revision": 2, "raw_ai_prompt": "must not persist"},
            ),
        )
    assert invalid.value.category == ErrorCategory.ENGINE_INVARIANT_ERROR
    assert store.load_state("libera", "unregistered_nick", "alice")["state_revision"] == 1
    with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
        assert connection.execute("SELECT count(*) FROM action_results").fetchone()[0] == 1
        assert connection.execute("SELECT count(*) FROM game_audits").fetchone()[0] == 1
    store.close()


def test_identical_retry_replays_and_changed_request_conflicts(tmp_path: Path) -> None:
    store = GameStore.open(config(), repository_root=tmp_path)
    original = request(revision=0)
    calls = 0

    def transition(_):
        nonlocal calls
        calls += 1
        return result(1, {"state_revision": 1, "lifecycle": "active", "currency": 10})

    first = store.execute(original, transition)
    replay = store.execute(original, transition)
    assert calls == 1
    assert replay == first
    assert store.load_state("libera", "unregistered_nick", "alice")["state_revision"] == 1

    changed = request(
        revision=1,
        action="advance",
        idempotency_key=original.idempotency_key,
    )
    with pytest.raises(GameServiceError) as conflict:
        store.execute(changed, lambda _: pytest.fail("conflicting retries must not transition"))
    assert conflict.value.category == ErrorCategory.IDEMPOTENCY_CONFLICT
    assert store.load_state("libera", "unregistered_nick", "alice")["state_revision"] == 1
    store.close()


def test_stale_revision_and_concurrent_double_spend_reward_are_rejected(tmp_path: Path) -> None:
    store = GameStore.open(config(), repository_root=tmp_path)
    create_session(store)

    stale = request(revision=0, action="buy", arguments=(("item_id", "kit"), ("quantity", 1)))
    with pytest.raises(GameServiceError) as stale_error:
        store.execute(stale, lambda _: pytest.fail("stale action must not transition"))
    assert stale_error.value.category == ErrorCategory.STALE_REVISION
    assert stale_error.value.state_revision == 1

    def purchase(action_request: StoreActionRequest):
        def transition(state):
            assert state["currency"] >= 10
            return result(2, {
                **state,
                "state_revision": 2,
                "currency": state["currency"] - 10,
                "claimed_rewards": ["encounter-one"],
            }, category="purchase_and_reward")
        try:
            return store.execute(action_request, transition)
        except GameServiceError as exc:
            return exc

    requests = [request(revision=1, action="buy") for _ in range(2)]
    with ThreadPoolExecutor(max_workers=2) as executor:
        outcomes = list(executor.map(purchase, requests))
    assert sum(isinstance(value, AuthoritativeResult) for value in outcomes) == 1
    failures = [value for value in outcomes if isinstance(value, GameServiceError)]
    assert len(failures) == 1 and failures[0].category == ErrorCategory.STALE_REVISION
    state = store.load_state("libera", "unregistered_nick", "alice")
    assert state["currency"] == 0
    assert state["claimed_rewards"] == ["encounter-one"]
    assert state["state_revision"] == 2
    store.close()


def test_committed_state_reopens_and_damaged_session_isolated(tmp_path: Path) -> None:
    store = GameStore.open(config(), repository_root=tmp_path)
    create_session(store, "alice", 7)
    create_session(store, "bob", 19)
    store.close()

    reopened = GameStore.open(config(), repository_root=tmp_path)
    assert reopened.load_state("libera", "unregistered_nick", "bob")["currency"] == 19
    with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
        connection.execute(
            "UPDATE game_sessions SET state_json = '[]' WHERE identity_value = 'alice'"
        )
        connection.commit()

    damaged_request = request(revision=1, identity="alice", action="advance")
    with pytest.raises(GameServiceError) as damaged:
        reopened.execute(damaged_request, lambda _: pytest.fail("damaged state must fail closed"))
    assert damaged.value.category == ErrorCategory.RECOVERY_REQUIRED
    assert reopened.load_state("libera", "unregistered_nick", "bob")["currency"] == 19
    with pytest.raises(GameServiceError) as blocked:
        reopened.load_state("libera", "unregistered_nick", "alice")
    assert blocked.value.category == ErrorCategory.RECOVERY_REQUIRED

    with sqlite3.connect(tmp_path / "data" / "game.db") as connection:
        alice = connection.execute(
            "SELECT lifecycle, recovery_id FROM game_sessions WHERE identity_value = 'alice'"
        ).fetchone()
        assert alice[0] == "recovery_required" and alice[1].startswith("rec-")
        assert connection.execute("SELECT count(*) FROM recovery_metadata").fetchone()[0] == 1
        assert connection.execute(
            "SELECT state_json FROM game_sessions WHERE identity_value = 'bob'"
        ).fetchone()[0] != "[]"
    reopened.close()


def test_failed_migration_stays_dirty_and_never_runs_down_automatically(tmp_path: Path) -> None:
    migrations = tmp_path / "migrations"
    migrations.mkdir()
    (migrations / "001_broken.sql").write_text(
        "CREATE TABLE partial(value INTEGER);\nTHIS IS NOT SQL;\n",
        encoding="utf-8",
    )
    (migrations / "001_broken.down.sql").write_text(
        "CREATE TABLE destructive_down_ran(value INTEGER);\n",
        encoding="utf-8",
    )
    pool = SQLiteConnectionPool(config(), repository_root=tmp_path)
    runner = MigrationRunner(pool, directory=migrations)
    with pytest.raises(MigrationError, match="001_broken"):
        runner.migrate()
    state = runner.state()
    assert state.dirty and state.version == 1
    with pool.connection() as connection:
        names = {
            row[0] for row in connection.execute(
                "SELECT name FROM sqlite_master WHERE type = 'table'"
            )
        }
        assert "partial" not in names
        assert "destructive_down_ran" not in names
    pool.close()


def test_core_database_names_are_rejected(tmp_path: Path) -> None:
    unsafe = config().model_copy(update={"database_path": "data/bot.db"})
    with pytest.raises(UnsafeDatabasePath):
        SQLiteConnectionPool(unsafe, repository_root=tmp_path)
