"""Ordered hand-rolled migrations for the dedicated game schema."""
from __future__ import annotations

import re
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from .sqlite import SQLiteConnectionPool

_UP_PATTERN = re.compile(r"^(?P<version>[0-9]{3})_(?P<name>[a-z0-9_]+)\.sql$")


class MigrationError(RuntimeError):
    """Actionable migration failure; callers must keep game readiness false."""

    def __init__(self, message: str, *, version: int = 0, name: str = "") -> None:
        super().__init__(message)
        self.version = version
        self.name = name


@dataclass(frozen=True)
class Migration:
    version: int
    name: str
    up_path: Path
    down_path: Path


@dataclass(frozen=True)
class MigrationState:
    version: int
    dirty: bool
    latest_available: int


def discover_migrations(directory: Path | None = None) -> tuple[Migration, ...]:
    root = directory or Path(__file__).resolve().parents[1] / "migrations"
    migrations: list[Migration] = []
    seen: set[int] = set()
    for path in sorted(root.glob("*.sql")):
        match = _UP_PATTERN.fullmatch(path.name)
        if not match:
            continue
        version = int(match.group("version"))
        name = match.group("name")
        if version in seen:
            raise MigrationError(f"duplicate game migration version {version}", version=version, name=name)
        down_path = path.with_name(f"{path.stem}.down.sql")
        if not down_path.is_file():
            raise MigrationError(f"game migration {version:03d}_{name} has no down file", version=version, name=name)
        seen.add(version)
        migrations.append(Migration(version, name, path, down_path))
    expected = list(range(1, len(migrations) + 1))
    actual = [migration.version for migration in migrations]
    if actual != expected:
        raise MigrationError(f"game migrations must be contiguous from 001; found {actual}")
    return tuple(migrations)


def _statements(script: str) -> tuple[str, ...]:
    statements: list[str] = []
    buffer = ""
    for line in script.splitlines(keepends=True):
        buffer += line
        if sqlite3.complete_statement(buffer):
            statement = buffer.strip()
            if statement:
                statements.append(statement)
            buffer = ""
    if buffer.strip():
        raise MigrationError("migration contains an incomplete SQL statement")
    return tuple(statements)


class MigrationRunner:
    """Applies upward migrations only; down files are operator tools."""

    def __init__(
        self,
        pool: SQLiteConnectionPool,
        *,
        directory: Path | None = None,
    ) -> None:
        self._pool = pool
        self._migrations = discover_migrations(directory)

    @property
    def latest_version(self) -> int:
        return self._migrations[-1].version if self._migrations else 0

    @staticmethod
    def _bootstrap(connection: sqlite3.Connection) -> None:
        connection.execute(
            """CREATE TABLE IF NOT EXISTS schema_migrations (
                version INTEGER PRIMARY KEY,
                name TEXT NOT NULL,
                dirty INTEGER NOT NULL CHECK(dirty IN (0, 1)),
                applied_at TEXT
            )"""
        )

    def state(self) -> MigrationState:
        with self._pool.connection() as connection:
            self._bootstrap(connection)
            dirty = connection.execute(
                "SELECT version, name FROM schema_migrations WHERE dirty = 1 ORDER BY version LIMIT 1"
            ).fetchone()
            clean = connection.execute(
                "SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = 0"
            ).fetchone()[0]
            return MigrationState(
                version=int(dirty[0] if dirty else clean),
                dirty=dirty is not None,
                latest_available=self.latest_version,
            )

    def migrate(self) -> MigrationState:
        with self._pool.connection() as connection:
            self._bootstrap(connection)
            dirty = connection.execute(
                "SELECT version, name FROM schema_migrations WHERE dirty = 1 ORDER BY version LIMIT 1"
            ).fetchone()
            if dirty:
                raise MigrationError(
                    f"game schema is dirty at migration {dirty['version']:03d}_{dirty['name']}; "
                    "inspect the database and restore or resolve it manually",
                    version=int(dirty["version"]),
                    name=str(dirty["name"]),
                )
            current = int(connection.execute(
                "SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = 0"
            ).fetchone()[0])
            if current > self.latest_version:
                raise MigrationError(
                    f"game database schema {current} is newer than supported {self.latest_version}",
                    version=current,
                )

            for migration in self._migrations:
                if migration.version <= current:
                    continue
                self._apply(connection, migration)
                current = migration.version
            return MigrationState(current, False, self.latest_version)

    @staticmethod
    def _apply(connection: sqlite3.Connection, migration: Migration) -> None:
        # The committed dirty marker survives a rolled-back migration and gates
        # readiness until an operator has inspected the failure.
        connection.execute("BEGIN IMMEDIATE")
        try:
            connection.execute(
                "INSERT INTO schema_migrations(version, name, dirty, applied_at) VALUES (?, ?, 1, NULL)",
                (migration.version, migration.name),
            )
            connection.commit()
        except Exception:
            connection.rollback()
            raise

        try:
            script = migration.up_path.read_text(encoding="utf-8")
            connection.execute("BEGIN IMMEDIATE")
            for statement in _statements(script):
                connection.execute(statement)
            connection.execute(
                "UPDATE schema_migrations SET dirty = 0, applied_at = ? WHERE version = ? AND dirty = 1",
                (datetime.now(timezone.utc).isoformat(), migration.version),
            )
            connection.commit()
        except Exception as exc:
            connection.rollback()
            raise MigrationError(
                f"failed game migration {migration.version:03d}_{migration.name}: {exc}",
                version=migration.version,
                name=migration.name,
            ) from exc


__all__ = [
    "Migration",
    "MigrationError",
    "MigrationRunner",
    "MigrationState",
    "discover_migrations",
]
