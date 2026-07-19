"""Dedicated game persistence; no Go or core bot database dependency."""
from .maintenance import GameMaintenance, MaintenanceResult
from .migrations import MigrationError, MigrationRunner, MigrationState
from .repository import GameStore, canonical_json, canonical_request_hash
from .sqlite import (
    DatabasePoolTimeout,
    GameDatabaseError,
    SQLiteConnectionPool,
    UnsafeDatabasePath,
    resolve_game_database_path,
)

__all__ = [
    "DatabasePoolTimeout",
    "GameDatabaseError",
    "GameMaintenance",
    "GameStore",
    "MaintenanceResult",
    "MigrationError",
    "MigrationRunner",
    "MigrationState",
    "SQLiteConnectionPool",
    "UnsafeDatabasePath",
    "canonical_json",
    "canonical_request_hash",
    "resolve_game_database_path",
]
