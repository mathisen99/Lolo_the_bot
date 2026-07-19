CREATE TABLE game_sessions (
    session_id TEXT PRIMARY KEY NOT NULL,
    network_id TEXT NOT NULL CHECK(length(network_id) BETWEEN 1 AND 32 AND network_id = lower(network_id)),
    identity_kind TEXT NOT NULL CHECK(identity_kind IN ('registered_user', 'unregistered_nick')),
    identity_value TEXT NOT NULL CHECK(length(identity_value) BETWEEN 1 AND 128),
    display_nick TEXT NOT NULL CHECK(length(display_nick) BETWEEN 1 AND 64),
    lifecycle TEXT NOT NULL CHECK(lifecycle IN ('active', 'completed', 'failed', 'recovery_required')),
    state_revision INTEGER NOT NULL CHECK(state_revision >= 0),
    state_json TEXT NOT NULL CHECK(json_valid(state_json)),
    state_schema_version INTEGER NOT NULL CHECK(state_schema_version >= 1),
    engine_version TEXT NOT NULL CHECK(length(engine_version) BETWEEN 1 AND 32),
    content_version TEXT NOT NULL CHECK(length(content_version) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_active_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    recovery_id TEXT,
    UNIQUE(network_id, identity_kind, identity_value),
    CHECK(identity_kind != 'registered_user' OR identity_value NOT GLOB '*[^0-9]*'),
    CHECK(identity_kind != 'unregistered_nick' OR identity_value = lower(identity_value))
);

CREATE TRIGGER game_sessions_revision_monotonic
BEFORE UPDATE OF state_revision ON game_sessions
WHEN NEW.state_revision < OLD.state_revision OR NEW.state_revision > OLD.state_revision + 1
BEGIN
    SELECT RAISE(ABORT, 'state_revision must stay equal or increase by exactly one');
END;

CREATE TABLE recovery_metadata (
    recovery_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT REFERENCES game_sessions(session_id) ON DELETE SET NULL,
    schema_version INTEGER NOT NULL CHECK(schema_version >= 0),
    engine_version TEXT NOT NULL,
    content_version TEXT NOT NULL,
    error_category TEXT NOT NULL,
    preserved_state_json TEXT,
    created_at TEXT NOT NULL,
    resolved_at TEXT
);

CREATE INDEX recovery_metadata_session_idx ON recovery_metadata(session_id, resolved_at);
