"""Secure, Python-owned SQLite connections for the dedicated game database."""
from __future__ import annotations

import os
import queue
import sqlite3
from contextlib import contextmanager
from pathlib import Path, PurePath
from threading import Lock
from typing import Iterator

from ..config import GameConfigSnapshot


class GameDatabaseError(RuntimeError):
    """Base error for game-owned database resources."""


class UnsafeDatabasePath(GameDatabaseError):
    pass


class DatabasePoolTimeout(GameDatabaseError):
    pass


def resolve_game_database_path(value: str, *, repository_root: Path | None = None) -> Path:
    """Resolve a normalized path below this repository's data directory.

    URI filenames, traversal, symlinks escaping ``data/``, and reuse of core
    databases are rejected before SQLite sees the value.
    """
    root = (repository_root or Path(__file__).resolve().parents[3]).resolve()
    relative = PurePath(value)
    if (
        not value
        or relative.is_absolute()
        or relative.parts[0] != "data"
        or ".." in relative.parts
        or "://" in value
        or "\x00" in value
        or not value.endswith(".db")
        or relative.name in {"bot.db", "reminders.db", "bugs.db"}
    ):
        raise UnsafeDatabasePath("game database must be a dedicated local .db path beneath data/")

    data_root = (root / "data").resolve()
    candidate = (root / Path(*relative.parts)).resolve(strict=False)
    try:
        candidate.relative_to(data_root)
    except ValueError as exc:
        raise UnsafeDatabasePath("game database path escapes the repository data directory") from exc
    if candidate == data_root:
        raise UnsafeDatabasePath("game database path must name a file")
    return candidate


class SQLiteConnectionPool:
    """Small bounded pool configured identically on every connection."""

    def __init__(
        self,
        config: GameConfigSnapshot,
        *,
        repository_root: Path | None = None,
    ) -> None:
        self.path = resolve_game_database_path(config.database_path, repository_root=repository_root)
        self.busy_timeout_ms = config.database_busy_timeout_ms
        self.max_size = config.database_pool_size
        self._available: queue.LifoQueue[sqlite3.Connection] = queue.LifoQueue(self.max_size)
        self._state_lock = Lock()
        self._created = 0
        self._closed = False
        self._prepare_local_storage()

    def _prepare_local_storage(self) -> None:
        parent = self.path.parent
        parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if parent.is_symlink():
            raise UnsafeDatabasePath("game database parent may not be a symbolic link")
        try:
            parent.chmod(0o700)
        except PermissionError as exc:
            raise UnsafeDatabasePath("game database directory permissions cannot be secured") from exc

    def _new_connection(self) -> sqlite3.Connection:
        connection = sqlite3.connect(
            self.path,
            timeout=self.busy_timeout_ms / 1000,
            isolation_level=None,
            check_same_thread=False,
        )
        connection.row_factory = sqlite3.Row
        try:
            mode = connection.execute("PRAGMA journal_mode=WAL").fetchone()[0]
            connection.execute("PRAGMA foreign_keys=ON")
            connection.execute(f"PRAGMA busy_timeout={int(self.busy_timeout_ms)}")
            connection.execute("PRAGMA synchronous=FULL")
            if str(mode).lower() != "wal":
                raise GameDatabaseError("game database did not enter WAL mode")
            if connection.execute("PRAGMA foreign_keys").fetchone()[0] != 1:
                raise GameDatabaseError("game database foreign keys could not be enabled")
        except Exception:
            connection.close()
            raise
        try:
            os.chmod(self.path, 0o600)
        except PermissionError:
            connection.close()
            raise UnsafeDatabasePath("game database file permissions cannot be secured")
        return connection

    def acquire(self) -> sqlite3.Connection:
        with self._state_lock:
            if self._closed:
                raise GameDatabaseError("game database pool is closed")
            try:
                return self._available.get_nowait()
            except queue.Empty:
                if self._created < self.max_size:
                    self._created += 1
                    create = True
                else:
                    create = False
        if create:
            try:
                return self._new_connection()
            except Exception:
                with self._state_lock:
                    self._created -= 1
                raise
        try:
            return self._available.get(timeout=self.busy_timeout_ms / 1000)
        except queue.Empty as exc:
            raise DatabasePoolTimeout("timed out waiting for a game database connection") from exc

    def release(self, connection: sqlite3.Connection) -> None:
        with self._state_lock:
            closed = self._closed
            if closed:
                self._created -= 1
        if closed:
            connection.close()
        else:
            self._available.put_nowait(connection)

    @contextmanager
    def connection(self) -> Iterator[sqlite3.Connection]:
        connection = self.acquire()
        try:
            yield connection
        finally:
            if connection.in_transaction:
                connection.rollback()
            self.release(connection)

    def close(self) -> None:
        with self._state_lock:
            self._closed = True
        while True:
            try:
                connection = self._available.get_nowait()
            except queue.Empty:
                break
            connection.close()
            with self._state_lock:
                self._created -= 1


__all__ = [
    "DatabasePoolTimeout",
    "GameDatabaseError",
    "SQLiteConnectionPool",
    "UnsafeDatabasePath",
    "resolve_game_database_path",
]
