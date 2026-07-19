"""Transactional repositories and authoritative game-store execution."""
from __future__ import annotations

import base64
import dataclasses
import hashlib
import json
import re
import secrets
import sqlite3
from collections import OrderedDict
from collections.abc import Callable, Iterator, Mapping
from contextlib import contextmanager
from datetime import datetime, timedelta, timezone
from enum import Enum
from pathlib import Path
from threading import Lock
from typing import Any
from uuid import UUID, uuid4

from pydantic import BaseModel

from ..application import (
    AuthoritativeChoice,
    AuthoritativeResult,
    GameServiceError,
    StoreActionRequest,
)
from ..config import GameConfigSnapshot
from ..models.api import ErrorCategory, LifecycleRequest
from .migrations import MigrationError, MigrationRunner, MigrationState
from .sqlite import (
    DatabasePoolTimeout,
    GameDatabaseError,
    SQLiteConnectionPool,
)

_NETWORK_RE = re.compile(r"^[a-z0-9_-]{1,32}$")
_FORBIDDEN_STATE_KEYS = {
    "hostmask", "password", "verification_password", "raw_ai_prompt",
    "pm_text", "raw_pm_text", "conversation", "narration",
}


def _jsonable(value: Any) -> Any:
    if isinstance(value, BaseModel):
        return _jsonable(value.model_dump(mode="json"))
    if dataclasses.is_dataclass(value) and not isinstance(value, type):
        return _jsonable(dataclasses.asdict(value))
    if isinstance(value, Mapping):
        result: dict[str, Any] = {}
        for key, item in value.items():
            if not isinstance(key, str):
                raise ValueError("persistent state object keys must be strings")
            if key.casefold() in _FORBIDDEN_STATE_KEYS:
                raise ValueError(f"privacy-prohibited field in persistent state: {key}")
            result[key] = _jsonable(item)
        return result
    if isinstance(value, (list, tuple)):
        return [_jsonable(item) for item in value]
    if isinstance(value, (set, frozenset)):
        converted = [_jsonable(item) for item in value]
        return sorted(converted, key=canonical_json)
    if isinstance(value, (datetime, UUID, Enum)):
        raw = value.isoformat() if isinstance(value, datetime) else value.value if isinstance(value, Enum) else str(value)
        return raw
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    raise ValueError(f"unsupported persistent value type: {type(value).__name__}")


def canonical_json(value: Any) -> str:
    return json.dumps(
        _jsonable(value), ensure_ascii=False, allow_nan=False,
        sort_keys=True, separators=(",", ":"),
    )


def canonical_request_hash(request: StoreActionRequest) -> bytes:
    payload = {
        "request_id": str(request.request_id),
        "idempotency_key": str(request.idempotency_key),
        "identity": {
            "network_id": request.network_id,
            "kind": request.identity_kind,
            "value": request.identity_value,
        },
        "expected_state_revision": request.expected_state_revision,
        "action": {
            "name": request.action.name,
            "arguments": dict(request.action.arguments),
            "menu_context_id": request.action.menu_context_id,
            "choice_token": request.action.choice_token,
        },
        "configuration_revision": request.configuration_revision,
        "content_policy_revision": request.content_policy_revision,
        "engine_version": request.engine_version,
        "content_version": request.content_version,
        "state_schema_version": request.state_schema_version,
    }
    return hashlib.sha256(canonical_json(payload).encode("utf-8")).digest()


def _result_payload(result: AuthoritativeResult) -> str:
    payload = {
        "result_category": result.result_category,
        "state_revision": result.state_revision,
        "state_changed": result.state_changed,
        "facts": list(result.facts),
        "choices": [
            {
                "input": choice.input,
                "kind": choice.kind,
                "action": choice.action,
                "arguments": dict(choice.arguments),
                "choice_token": choice.choice_token,
            }
            for choice in result.choices
        ],
        "menu_context_id": result.menu_context_id,
        "menu_page": result.menu_page,
        "menu_expires_at": (
            result.menu_expires_at.isoformat()
            if isinstance(result.menu_expires_at, datetime)
            else result.menu_expires_at
        ),
        "milestones": list(result.milestones),
        "delivery_target": result.delivery_target,
        "random_metadata": dict(result.random_metadata),
    }
    return canonical_json(payload)


def _result_from_payload(value: str) -> AuthoritativeResult:
    payload = json.loads(value)
    expires = payload.get("menu_expires_at")
    if expires is not None:
        expires = datetime.fromisoformat(expires)
    return AuthoritativeResult(
        result_category=payload["result_category"],
        state_revision=int(payload["state_revision"]),
        state_changed=bool(payload["state_changed"]),
        facts=tuple(payload["facts"]),
        choices=tuple(
            AuthoritativeChoice(
                input=choice["input"],
                kind=choice["kind"],
                action=choice["action"],
                arguments=tuple(sorted(choice["arguments"].items())),
                choice_token=choice["choice_token"],
            )
            for choice in payload["choices"]
        ),
        menu_context_id=payload.get("menu_context_id"),
        menu_page=int(payload["menu_page"]),
        menu_expires_at=expires,
        milestones=tuple(payload["milestones"]),
        delivery_target=payload["delivery_target"],
        random_metadata=tuple(sorted(payload.get("random_metadata", {}).items())),
    )


def _identity_key(network_id: str, kind: str, value: str) -> tuple[str, str, str]:
    if not _NETWORK_RE.fullmatch(network_id) or kind not in {"registered_user", "unregistered_nick"}:
        raise GameServiceError(ErrorCategory.IDENTITY_AMBIGUOUS, "Game identity is invalid.")
    if not value or len(value.encode("utf-8")) > 128 or any(char in value for char in "\x00\r\n"):
        raise GameServiceError(ErrorCategory.IDENTITY_AMBIGUOUS, "Game identity is invalid.")
    if (
        kind == "registered_user"
        and (not value.isascii() or not value.isdigit())
    ):
        raise GameServiceError(ErrorCategory.IDENTITY_AMBIGUOUS, "Registered game identity is invalid.")
    if kind == "unregistered_nick" and value != value.lower():
        raise GameServiceError(ErrorCategory.IDENTITY_AMBIGUOUS, "Nickname game identity is not canonical.")
    return network_id, kind, value


def _session_ref_hash(identity: tuple[str, str, str]) -> bytes:
    return hashlib.sha256("\x1f".join(identity).encode("utf-8")).digest()


def _reset_token_hash(token: str) -> bytes:
    return hashlib.sha256(token.encode("ascii")).digest()


def _new_reset_token() -> str:
    return "r-" + base64.b32encode(secrets.token_bytes(16)).decode("ascii").rstrip("=").lower()


def _bind_fresh_menu_context(result: AuthoritativeResult) -> AuthoritativeResult:
    """Bind opaque choices to one fresh interaction context.

    Campaign state and revision are untouched. Re-rendering or pagination gets a
    new context and therefore new opaque tokens, while stable named actions
    remain discoverable.
    """
    context_id = f"m-{uuid4().hex}"
    choices: list[AuthoritativeChoice] = []
    for ordinal, choice in enumerate(result.choices):
        if choice.kind == "choice":
            digest = hashlib.sha256(f"{context_id}:{ordinal}".encode("ascii")).digest()[:5]
            token = "c-" + base64.b32encode(digest).decode("ascii").rstrip("=").lower()
            choice = dataclasses.replace(choice, input=token, choice_token=token)
        choices.append(choice)
    return dataclasses.replace(result, menu_context_id=context_id, choices=tuple(choices))


def _revision_may_be_discovered(request: StoreActionRequest) -> bool:
    return request.expected_state_revision == 0 and request.action.name in {
        "start", "resume", "status", "inventory", "help", "credits", "privacy", "content",
    }


def _utcnow() -> datetime:
    return datetime.now(timezone.utc)


class IdentityLockRegistry:
    """Bounded per-canonical-identity serialization for in-process callers."""

    def __init__(self, maximum: int) -> None:
        self._maximum = maximum
        self._guard = Lock()
        self._entries: OrderedDict[tuple[str, str, str], tuple[Lock, int]] = OrderedDict()

    @contextmanager
    def hold(self, key: tuple[str, str, str]) -> Iterator[None]:
        with self._guard:
            entry = self._entries.get(key)
            if entry is None:
                while len(self._entries) >= self._maximum:
                    removable = next((candidate for candidate, (_, users) in self._entries.items() if users == 0), None)
                    if removable is None:
                        raise DatabasePoolTimeout("game identity lock capacity is exhausted")
                    del self._entries[removable]
                lock, users = Lock(), 0
            else:
                lock, users = entry
            self._entries[key] = (lock, users + 1)
            self._entries.move_to_end(key)
        lock.acquire()
        try:
            yield
        finally:
            lock.release()
            with self._guard:
                current_lock, users = self._entries[key]
                self._entries[key] = (current_lock, users - 1)


class SessionRepository:
    @staticmethod
    def find(connection: sqlite3.Connection, identity: tuple[str, str, str]) -> sqlite3.Row | None:
        return connection.execute(
            """SELECT * FROM game_sessions
               WHERE network_id = ? AND identity_kind = ? AND identity_value = ?""",
            identity,
        ).fetchone()

    @staticmethod
    def insert(
        connection: sqlite3.Connection,
        request: StoreActionRequest,
        identity: tuple[str, str, str],
        state_json: str,
        result: AuthoritativeResult,
        now: datetime,
        retention_days: int,
    ) -> str:
        session_id = str(uuid4())
        timestamp = now.isoformat()
        connection.execute(
            """INSERT INTO game_sessions(
                session_id, network_id, identity_kind, identity_value, display_nick,
                lifecycle, state_revision, state_json, state_schema_version,
                engine_version, content_version, created_at, updated_at,
                last_active_at, expires_at, recovery_id
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)""",
            (
                session_id, *identity, request.display_nick,
                _lifecycle(json.loads(state_json)), result.state_revision, state_json,
                request.state_schema_version, request.engine_version, request.content_version,
                timestamp, timestamp, timestamp,
                (now + timedelta(days=retention_days)).isoformat(),
            ),
        )
        connection.execute(
            """INSERT INTO content_preferences(
                session_id, selected_profile, adult_opt_in, milestone_opt_in,
                category_restrictions_json, policy_revision, updated_at
            ) VALUES (?, 'standard', 0, 0, '{}', ?, ?)""",
            (session_id, request.content_policy_revision, timestamp),
        )
        return session_id

    @staticmethod
    def update(
        connection: sqlite3.Connection,
        row: sqlite3.Row,
        request: StoreActionRequest,
        state_json: str,
        result: AuthoritativeResult,
        now: datetime,
        retention_days: int,
    ) -> None:
        cursor = connection.execute(
            """UPDATE game_sessions SET
                display_nick = ?, lifecycle = ?, state_revision = ?, state_json = ?,
                state_schema_version = ?, engine_version = ?, content_version = ?,
                updated_at = ?, last_active_at = ?, expires_at = ?
               WHERE session_id = ? AND state_revision = ? AND recovery_id IS NULL""",
            (
                request.display_nick, _lifecycle(json.loads(state_json)), result.state_revision,
                state_json, request.state_schema_version, request.engine_version,
                request.content_version, now.isoformat(), now.isoformat(),
                (now + timedelta(days=retention_days)).isoformat(), row["session_id"],
                request.expected_state_revision,
            ),
        )
        if cursor.rowcount != 1:
            raise GameServiceError(
                ErrorCategory.STALE_REVISION,
                "Game state changed; refresh the current menu.",
                state_revision=int(row["state_revision"]),
            )


class ActionRepository:
    @staticmethod
    def find_existing(
        connection: sqlite3.Connection,
        identity: tuple[str, str, str],
        request: StoreActionRequest,
    ) -> sqlite3.Row | None:
        return connection.execute(
            """SELECT ar.*, gs.network_id, gs.identity_kind, gs.identity_value
               FROM action_results ar JOIN game_sessions gs ON gs.session_id = ar.session_id
               WHERE ar.request_id = ? OR (
                   gs.network_id = ? AND gs.identity_kind = ? AND gs.identity_value = ?
                   AND ar.idempotency_key = ?)
               ORDER BY CASE WHEN ar.request_id = ? THEN 0 ELSE 1 END LIMIT 1""",
            (
                str(request.request_id), *identity, str(request.idempotency_key),
                str(request.request_id),
            ),
        ).fetchone()

    @staticmethod
    def insert(
        connection: sqlite3.Connection,
        session_id: str,
        request: StoreActionRequest,
        request_hash: bytes,
        pre_revision: int,
        result: AuthoritativeResult,
        now: datetime,
    ) -> None:
        connection.execute(
            """INSERT INTO action_results(
                action_record_id, session_id, request_id, idempotency_key, request_hash,
                action_type, pre_revision, post_revision, state_changed, result_json,
                result_category, random_metadata_json, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                str(uuid4()), session_id, str(request.request_id), str(request.idempotency_key),
                request_hash, request.action.name, pre_revision, result.state_revision,
                int(result.state_changed), _result_payload(result), result.result_category,
                canonical_json(dict(result.random_metadata)), now.isoformat(),
            ),
        )


class MenuRepository:
    @staticmethod
    def replace(
        connection: sqlite3.Connection,
        session_id: str,
        request: StoreActionRequest,
        result: AuthoritativeResult,
        now: datetime,
    ) -> None:
        connection.execute(
            "UPDATE menu_contexts SET superseded_at = ? WHERE session_id = ? AND superseded_at IS NULL",
            (now.isoformat(), session_id),
        )
        if not result.choices:
            return
        if result.menu_context_id is None or result.menu_expires_at is None:
            raise ValueError("choices require a persisted menu context")
        expires = (
            result.menu_expires_at.isoformat()
            if isinstance(result.menu_expires_at, datetime)
            else str(result.menu_expires_at)
        )
        connection.execute(
            """INSERT INTO menu_contexts(
                context_id, session_id, state_revision, page, context_version,
                content_policy_revision, created_at, expires_at, superseded_at
            ) VALUES (?, ?, ?, ?, 1, ?, ?, ?, NULL)""",
            (
                result.menu_context_id, session_id, result.state_revision, result.menu_page,
                request.content_policy_revision, now.isoformat(), expires,
            ),
        )
        for ordinal, choice in enumerate(result.choices):
            token_hash = hashlib.sha256(choice.input.encode("utf-8")).digest()
            connection.execute(
                """INSERT INTO menu_choices(
                    context_id, token_hash, display_token, action_name, arguments_json, ordinal
                ) VALUES (?, ?, ?, ?, ?, ?)""",
                (
                    result.menu_context_id, token_hash, choice.input, choice.action,
                    canonical_json(dict(choice.arguments)), ordinal,
                ),
            )


class AuditRepository:
    @staticmethod
    def insert(
        connection: sqlite3.Connection,
        request_id: UUID,
        identity: tuple[str, str, str],
        event_type: str,
        pre_revision: int | None,
        post_revision: int | None,
        result_category: str,
        details: Mapping[str, Any],
        now: datetime,
    ) -> None:
        connection.execute(
            """INSERT INTO game_audits(
                audit_id, request_id, session_ref_hash, event_type, pre_revision,
                post_revision, result_category, details_json, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                str(uuid4()), str(request_id), _session_ref_hash(identity), event_type,
                pre_revision, post_revision, result_category,
                canonical_json(details), now.isoformat(),
            ),
        )


class ResetTokenRepository:
    """Hash-only reset confirmation storage bound to one session revision."""

    @staticmethod
    def issue(
        connection: sqlite3.Connection,
        session_id: str,
        identity: tuple[str, str, str],
        revision: int,
        request_id: UUID,
        now: datetime,
        ttl_seconds: int,
    ) -> str:
        # A newer confirmation supersedes every older unused confirmation for
        # this session. A transport retry with the same request ID replaces its
        # undisclosed prior value because plaintext confirmation tokens are
        # intentionally never persisted.
        connection.execute("DELETE FROM reset_tokens WHERE request_id = ?", (str(request_id),))
        connection.execute(
            "UPDATE reset_tokens SET used_at = ? WHERE session_id = ? AND used_at IS NULL",
            (now.isoformat(), session_id),
        )
        for _ in range(4):
            token = _new_reset_token()
            try:
                connection.execute(
                    """INSERT INTO reset_tokens(
                        token_hash, session_id, identity_key_hash, issued_revision,
                        expires_at, used_at, request_id
                    ) VALUES (?, ?, ?, ?, ?, NULL, ?)""",
                    (
                        _reset_token_hash(token), session_id, _session_ref_hash(identity),
                        revision, (now + timedelta(seconds=ttl_seconds)).isoformat(),
                        str(request_id),
                    ),
                )
                return token
            except sqlite3.IntegrityError as exc:
                if "token_hash" not in str(exc).lower():
                    raise
        raise GameDatabaseError("could not allocate a unique reset confirmation token")

    @staticmethod
    def consume(
        connection: sqlite3.Connection,
        session_id: str,
        identity: tuple[str, str, str],
        revision: int,
        token: str,
        now: datetime,
    ) -> None:
        try:
            token_hash = _reset_token_hash(token)
        except (UnicodeEncodeError, AttributeError) as exc:
            raise GameServiceError(
                ErrorCategory.STALE_CONTEXT,
                "Reset confirmation is invalid or expired; request a new token.",
                state_revision=revision,
            ) from exc
        row = connection.execute(
            "SELECT * FROM reset_tokens WHERE token_hash = ?",
            (token_hash,),
        ).fetchone()
        valid = row is not None
        if valid:
            try:
                expires_at = datetime.fromisoformat(str(row["expires_at"]))
                valid = (
                    row["session_id"] == session_id
                    and bytes(row["identity_key_hash"]) == _session_ref_hash(identity)
                    and int(row["issued_revision"]) == revision
                    and row["used_at"] is None
                    and expires_at > now
                )
            except (TypeError, ValueError):
                valid = False
        if not valid:
            raise GameServiceError(
                ErrorCategory.STALE_CONTEXT,
                "Reset confirmation is invalid or expired; request a new token.",
                state_revision=revision,
            )
        updated = connection.execute(
            "UPDATE reset_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL",
            (now.isoformat(), token_hash),
        )
        if updated.rowcount != 1:
            raise GameServiceError(
                ErrorCategory.STALE_CONTEXT,
                "Reset confirmation is invalid or expired; request a new token.",
                state_revision=revision,
            )


class SessionArchiveRepository:
    @staticmethod
    def insert(
        connection: sqlite3.Connection,
        row: sqlite3.Row,
        now: datetime,
        retention_days: int,
    ) -> None:
        connection.execute(
            """INSERT INTO session_archives(
                archive_id, session_id, prior_revision, state_ciphertext_or_json,
                reason, expires_at, created_at
            ) VALUES (?, ?, ?, ?, 'reset', ?, ?)""",
            (
                str(uuid4()), row["session_id"], int(row["state_revision"]),
                row["state_json"], (now + timedelta(days=retention_days)).isoformat(),
                now.isoformat(),
            ),
        )


def _lifecycle(state: Mapping[str, Any]) -> str:
    value = state.get("lifecycle", "active")
    if value not in {"active", "completed", "failed", "recovery_required"}:
        raise ValueError("persistent state has an invalid lifecycle")
    return str(value)


def _default_validate_state(state: object, revision: int) -> None:
    if not isinstance(state, Mapping):
        raise ValueError("persistent session state must be an object")
    embedded_revision = state.get("state_revision")
    if embedded_revision is not None and embedded_revision != revision:
        raise ValueError("persistent state revision does not match the session revision")
    canonical_json(state)


class GameStore:
    """SQLite-backed implementation of the application ``GameStore`` protocol."""

    def __init__(
        self,
        pool: SQLiteConnectionPool,
        config: GameConfigSnapshot,
        *,
        migrations: MigrationRunner | None = None,
        invariant_validator: Callable[[object, int], None] | None = None,
        now_factory: Callable[[], datetime] | None = None,
    ) -> None:
        self._pool = pool
        self._config = config
        self._migrations = migrations or MigrationRunner(pool)
        self._validate_state = invariant_validator or _default_validate_state
        self._now_factory = now_factory or _utcnow
        self._locks = IdentityLockRegistry(config.max_continuation_identities)
        self._ready = False
        self._migration_state = MigrationState(0, False, self._migrations.latest_version)

    @classmethod
    def open(
        cls,
        config: GameConfigSnapshot,
        *,
        repository_root: Path | None = None,
        migrations_directory: Path | None = None,
        invariant_validator: Callable[[object, int], None] | None = None,
        now_factory: Callable[[], datetime] | None = None,
    ) -> "GameStore":
        pool = SQLiteConnectionPool(config, repository_root=repository_root)
        store = cls(
            pool, config,
            migrations=MigrationRunner(pool, directory=migrations_directory),
            invariant_validator=invariant_validator,
            now_factory=now_factory,
        )
        try:
            store._migration_state = store._migrations.migrate()
            store._ready = True
            return store
        except Exception:
            pool.close()
            raise

    @property
    def schema_version(self) -> int:
        return self._migration_state.version

    @property
    def ready(self) -> bool:
        return self._ready and not self._migration_state.dirty

    def close(self) -> None:
        self._ready = False
        self._pool.close()

    def _require_ready(self) -> None:
        if not self.ready:
            raise GameServiceError(
                ErrorCategory.MIGRATION_FAILED,
                "Game storage schema is not ready; an operator must inspect migrations.",
                retryable=True,
            )

    def execute(
        self,
        request: StoreActionRequest,
        transition: Callable[[object | None], AuthoritativeResult],
    ) -> AuthoritativeResult:
        self._require_ready()
        identity = _identity_key(request.network_id, request.identity_kind, request.identity_value)
        request_hash = canonical_request_hash(request)
        try:
            with self._locks.hold(identity), self._pool.connection() as connection:
                return self._execute_transaction(connection, identity, request, request_hash, transition)
        except GameServiceError:
            raise
        except ValueError as exc:
            raise GameServiceError(
                ErrorCategory.ENGINE_INVARIANT_ERROR,
                "The game rejected an invalid state transition.",
            ) from exc
        except (DatabasePoolTimeout, sqlite3.OperationalError) as exc:
            raise GameServiceError(
                ErrorCategory.DATABASE_BUSY,
                "Game storage is busy; retry shortly.",
                retryable=True,
            ) from exc
        except (sqlite3.DatabaseError, GameDatabaseError) as exc:
            raise GameServiceError(
                ErrorCategory.DATABASE_UNAVAILABLE,
                "Game storage is temporarily unavailable.",
                retryable=True,
            ) from exc

    def _execute_transaction(
        self,
        connection: sqlite3.Connection,
        identity: tuple[str, str, str],
        request: StoreActionRequest,
        request_hash: bytes,
        transition: Callable[[object | None], AuthoritativeResult],
    ) -> AuthoritativeResult:
        connection.execute("PRAGMA synchronous=FULL")
        connection.execute("BEGIN IMMEDIATE")
        try:
            existing = ActionRepository.find_existing(connection, identity, request)
            if existing is not None:
                exact_identity = (
                    existing["network_id"], existing["identity_kind"], existing["identity_value"]
                ) == identity
                same_keys = (
                    existing["request_id"] == str(request.request_id)
                    and existing["idempotency_key"] == str(request.idempotency_key)
                )
                if exact_identity and same_keys and existing["request_hash"] == request_hash:
                    replay = _result_from_payload(existing["result_json"])
                    connection.commit()
                    return replay
                raise GameServiceError(
                    ErrorCategory.IDEMPOTENCY_CONFLICT,
                    "The request identifier was already used for a different game action.",
                    state_revision=int(existing["post_revision"]) if exact_identity else 0,
                )

            row = SessionRepository.find(connection, identity)
            if row is not None and row["recovery_id"] is not None:
                raise GameServiceError(
                    ErrorCategory.RECOVERY_REQUIRED,
                    "This session requires operator recovery before it can change.",
                    state_revision=int(row["state_revision"]),
                )
            current_revision = int(row["state_revision"]) if row is not None else 0
            if request.expected_state_revision != current_revision and not _revision_may_be_discovered(request):
                raise GameServiceError(
                    ErrorCategory.STALE_REVISION,
                    "Game state changed; refresh the current menu.",
                    state_revision=current_revision,
                )

            state: object | None = None
            if row is not None:
                try:
                    state = json.loads(row["state_json"])
                    self._validate_state(state, current_revision)
                except Exception as exc:
                    recovery_id = self._record_recovery(connection, row, identity, request, exc)
                    connection.commit()
                    raise GameServiceError(
                        ErrorCategory.RECOVERY_REQUIRED,
                        f"This session requires operator recovery ({recovery_id}).",
                        state_revision=current_revision,
                    ) from exc

            now = self._now_factory()
            reset_token = request.action.argument_dict().get("token") if request.action.name == "reset" else None
            if request.action.name == "reset" and not reset_token:
                if row is None:
                    raise GameServiceError(
                        ErrorCategory.INVALID_INPUT,
                        "Start a campaign before requesting a reset.",
                    )
                token = ResetTokenRepository.issue(
                    connection, str(row["session_id"]), identity, current_revision,
                    request.request_id, now, self._config.reset_confirmation_ttl_seconds,
                )
                connection.execute(
                    "UPDATE menu_contexts SET superseded_at = ? WHERE session_id = ? AND superseded_at IS NULL",
                    (now.isoformat(), row["session_id"]),
                )
                connection.execute(
                    "UPDATE game_sessions SET display_nick = ?, last_active_at = ?, expires_at = ? WHERE session_id = ?",
                    (
                        request.display_nick,
                        now.isoformat(),
                        (now + timedelta(days=self._config.save_retention_days)).isoformat(),
                        row["session_id"],
                    ),
                )
                result = AuthoritativeResult(
                    result_category="reset_confirmation_required",
                    state_revision=current_revision,
                    state_changed=False,
                    facts=("Reset requested; progress is unchanged until the confirmation is used.",),
                    choices=(AuthoritativeChoice(
                        input=token,
                        kind="confirmation",
                        action="reset",
                        arguments=(("token", token),),
                    ),),
                    menu_context_id=f"m-reset-{uuid4().hex[:16]}",
                    menu_expires_at=now + timedelta(
                        seconds=self._config.reset_confirmation_ttl_seconds,
                    ),
                )
                AuditRepository.insert(
                    connection, request.request_id, identity, "reset_confirmation_issued",
                    current_revision, current_revision, result.result_category,
                    {"expires_at": result.menu_expires_at.isoformat()}, now,
                )
                connection.commit()
                return result

            if request.action.name == "reset":
                if row is None or not isinstance(reset_token, str):
                    raise GameServiceError(
                        ErrorCategory.STALE_CONTEXT,
                        "Reset confirmation is invalid or expired; request a new token.",
                        state_revision=current_revision,
                    )
                ResetTokenRepository.consume(
                    connection, str(row["session_id"]), identity, current_revision,
                    reset_token, now,
                )

            result = transition(state)
            result = self._add_inactive_save_warning(row, request, result, now)
            if result.choices:
                # Menu contexts are interaction state, not campaign state. A
                # fresh context also rebinds every opaque choice token so no
                # page or help refresh can silently reuse an earlier token.
                result = _bind_fresh_menu_context(result)
            self._validate_result(request, row, result)
            if request.action.name == "reset":
                assert row is not None
                SessionArchiveRepository.insert(
                    connection, row, now, self._config.reset_archive_retention_days,
                )
            if result.state_changed:
                self._validate_state(result.next_state, result.state_revision)
                state_json = canonical_json(result.next_state)
                if row is None:
                    session_id = SessionRepository.insert(
                        connection, request, identity, state_json, result, now,
                        self._config.save_retention_days,
                    )
                else:
                    SessionRepository.update(
                        connection, row, request, state_json, result, now,
                        self._config.save_retention_days,
                    )
                    session_id = str(row["session_id"])
            else:
                if row is None:
                    # Discovery/help before start is intentionally ephemeral:
                    # it creates no player session, action record, audit, or
                    # campaign revision, but can still offer a bounded start
                    # continuation to the current IRC interaction.
                    connection.commit()
                    return result
                session_id = str(row["session_id"])
                connection.execute(
                    "UPDATE game_sessions SET display_nick = ?, last_active_at = ?, expires_at = ? WHERE session_id = ?",
                    (
                        request.display_nick,
                        now.isoformat(),
                        (now + timedelta(days=self._config.save_retention_days)).isoformat(),
                        session_id,
                    ),
                )

            MenuRepository.replace(connection, session_id, request, result, now)
            for milestone in result.milestones:
                connection.execute(
                    """INSERT OR IGNORE INTO milestones(
                        milestone_id, session_id, milestone_key, event_type, state_revision,
                        announcement_allowed, announced_at, created_at
                    ) VALUES (?, ?, ?, ?, ?, 0, NULL, ?)""",
                    (str(uuid4()), session_id, milestone, milestone, result.state_revision, now.isoformat()),
                )
            ActionRepository.insert(
                connection, session_id, request, request_hash, current_revision, result, now,
            )
            AuditRepository.insert(
                connection, request.request_id, identity, request.action.name,
                current_revision, result.state_revision, result.result_category,
                {
                    "configuration_revision": request.configuration_revision,
                    "content_policy_revision": request.content_policy_revision,
                    "state_changed": result.state_changed,
                },
                now,
            )
            connection.commit()
            return result
        except GameServiceError:
            if connection.in_transaction:
                connection.rollback()
            raise
        except Exception:
            if connection.in_transaction:
                connection.rollback()
            raise

    def _add_inactive_save_warning(
        self,
        row: sqlite3.Row | None,
        request: StoreActionRequest,
        result: AuthoritativeResult,
        now: datetime,
    ) -> AuthoritativeResult:
        if row is None or request.action.name != "status":
            return result
        try:
            expires_at = datetime.fromisoformat(str(row["expires_at"]))
        except (TypeError, ValueError):
            return result
        warning_at = expires_at - timedelta(days=self._config.save_expiry_warning_days)
        if warning_at <= now < expires_at:
            remaining = max(1, (expires_at - now).days + 1)
            warning = (
                f"Inactive-save notice: this campaign was due for deletion in about {remaining} day(s); "
                "this status check renewed its retention window."
            )
            return dataclasses.replace(result, facts=(warning, *result.facts))
        return result

    @staticmethod
    def _validate_result(
        request: StoreActionRequest,
        row: sqlite3.Row | None,
        result: AuthoritativeResult,
    ) -> None:
        current = int(row["state_revision"]) if row is not None else 0
        expected_post = current + 1 if result.state_changed else current
        if result.state_revision != expected_post:
            raise GameServiceError(
                ErrorCategory.ENGINE_INVARIANT_ERROR,
                "The game rejected an invalid state transition.",
                state_revision=current,
            )
        if result.state_changed and result.next_state is None:
            raise GameServiceError(
                ErrorCategory.ENGINE_INVARIANT_ERROR,
                "The game rejected an incomplete state transition.",
                state_revision=current,
            )
        if request.expected_state_revision != current and not _revision_may_be_discovered(request):
            raise GameServiceError(
                ErrorCategory.STALE_REVISION,
                "Game state changed; refresh the current menu.",
                state_revision=current,
            )

    def _record_recovery(
        self,
        connection: sqlite3.Connection,
        row: sqlite3.Row,
        identity: tuple[str, str, str],
        request: StoreActionRequest,
        error: Exception,
    ) -> str:
        recovery_id = f"rec-{uuid4().hex[:16]}"
        now = self._now_factory()
        preserved = row["state_json"] if isinstance(row["state_json"], str) else None
        connection.execute(
            """INSERT INTO recovery_metadata(
                recovery_id, session_id, schema_version, engine_version, content_version,
                error_category, preserved_state_json, created_at, resolved_at
            ) VALUES (?, ?, ?, ?, ?, 'state_unreadable', ?, ?, NULL)""",
            (
                recovery_id, row["session_id"], row["state_schema_version"],
                row["engine_version"], row["content_version"], preserved, now.isoformat(),
            ),
        )
        connection.execute(
            "UPDATE game_sessions SET lifecycle = 'recovery_required', recovery_id = ? WHERE session_id = ?",
            (recovery_id, row["session_id"]),
        )
        AuditRepository.insert(
            connection, request.request_id, identity, "recovery_required",
            int(row["state_revision"]), int(row["state_revision"]), "state_unreadable",
            {"recovery_id": recovery_id, "error_type": type(error).__name__}, now,
        )
        return recovery_id

    def execute_lifecycle(self, request: LifecycleRequest) -> None:
        self._require_ready()
        identity = _identity_key(request.network_id, request.identity.kind, request.identity.value)
        with self._locks.hold(identity), self._pool.connection() as connection:
            connection.execute("PRAGMA synchronous=FULL")
            connection.execute("BEGIN IMMEDIATE")
            try:
                row = SessionRepository.find(connection, identity)
                now = self._now_factory()
                if request.operation == "invalidate_context":
                    if row is not None:
                        connection.execute(
                            "UPDATE menu_contexts SET superseded_at = ? WHERE session_id = ? AND superseded_at IS NULL",
                            (now.isoformat(), row["session_id"]),
                        )
                else:
                    assert request.new_identity is not None
                    target = _identity_key(
                        request.network_id, request.new_identity.kind, request.new_identity.value,
                    )
                    if identity[1] != "unregistered_nick" or target[1] != "unregistered_nick":
                        raise GameServiceError(
                            ErrorCategory.IDENTITY_AMBIGUOUS,
                            "Only observed nickname identities can be transferred automatically.",
                        )
                    if row is not None:
                        destination = SessionRepository.find(connection, target)
                        if destination is not None and destination["session_id"] != row["session_id"]:
                            raise GameServiceError(
                                ErrorCategory.IDENTITY_AMBIGUOUS,
                                "Both nickname identities own game sessions; operator recovery is required.",
                                state_revision=int(row["state_revision"]),
                            )
                        connection.execute(
                            "UPDATE game_sessions SET identity_value = ?, updated_at = ? WHERE session_id = ?",
                            (target[2], now.isoformat(), row["session_id"]),
                        )
                        AuditRepository.insert(
                            connection, request.request_id, identity, "identity_transferred",
                            int(row["state_revision"]), int(row["state_revision"]), "success",
                            {"target_session_ref": _session_ref_hash(target).hex()}, now,
                        )
                connection.commit()
            except Exception:
                connection.rollback()
                raise

    def run_maintenance(self, now: datetime | None = None) -> object:
        """Run one bounded cleanup without holding game identity locks."""
        self._require_ready()
        from .maintenance import GameMaintenance
        return GameMaintenance(self._pool, self._config).run_once(now)

    def delete_authenticated_session(self, network_id: str, identity_kind: str, identity_value: str) -> bool:
        """Trusted boundary for a caller that already authenticated deletion."""
        self._require_ready()
        identity = _identity_key(network_id, identity_kind, identity_value)
        from .maintenance import GameMaintenance
        with self._locks.hold(identity):
            return GameMaintenance(self._pool, self._config).delete_authenticated_session(*identity)

    def load_state(self, network_id: str, identity_kind: str, identity_value: str) -> object | None:
        """Load one exact session for service/tests without cross-identity fallback."""
        self._require_ready()
        identity = _identity_key(network_id, identity_kind, identity_value)
        with self._pool.connection() as connection:
            row = SessionRepository.find(connection, identity)
            if row is None:
                return None
            if row["recovery_id"] is not None:
                raise GameServiceError(
                    ErrorCategory.RECOVERY_REQUIRED,
                    "This session requires operator recovery before it can be loaded.",
                    state_revision=int(row["state_revision"]),
                )
            try:
                state = json.loads(row["state_json"])
                self._validate_state(state, int(row["state_revision"]))
                return state
            except Exception as exc:
                raise GameServiceError(
                    ErrorCategory.RECOVERY_REQUIRED,
                    "This session requires operator recovery before it can be loaded.",
                    state_revision=int(row["state_revision"]),
                ) from exc


__all__ = [
    "ActionRepository",
    "AuditRepository",
    "GameStore",
    "IdentityLockRegistry",
    "MenuRepository",
    "SessionRepository",
    "canonical_json",
    "canonical_request_hash",
]
