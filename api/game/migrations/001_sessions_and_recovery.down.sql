DROP INDEX IF EXISTS recovery_metadata_session_idx;
DROP TABLE IF EXISTS recovery_metadata;
DROP TRIGGER IF EXISTS game_sessions_revision_monotonic;
DROP TABLE IF EXISTS game_sessions;
