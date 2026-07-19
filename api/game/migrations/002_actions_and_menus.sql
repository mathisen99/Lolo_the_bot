CREATE TABLE menu_contexts (
    context_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES game_sessions(session_id) ON DELETE CASCADE,
    state_revision INTEGER NOT NULL CHECK(state_revision >= 0),
    page INTEGER NOT NULL CHECK(page >= 1),
    context_version INTEGER NOT NULL CHECK(context_version >= 1),
    content_policy_revision INTEGER NOT NULL CHECK(content_policy_revision >= 1),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    superseded_at TEXT
);

CREATE INDEX menu_contexts_session_idx ON menu_contexts(session_id, superseded_at, expires_at);

CREATE TABLE menu_choices (
    context_id TEXT NOT NULL REFERENCES menu_contexts(context_id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL CHECK(length(token_hash) = 32),
    display_token TEXT NOT NULL,
    action_name TEXT NOT NULL,
    arguments_json TEXT NOT NULL CHECK(json_valid(arguments_json)),
    ordinal INTEGER NOT NULL CHECK(ordinal >= 0),
    PRIMARY KEY(context_id, token_hash),
    UNIQUE(context_id, ordinal)
);

CREATE TABLE action_results (
    action_record_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES game_sessions(session_id) ON DELETE CASCADE,
    request_id TEXT NOT NULL UNIQUE,
    idempotency_key TEXT NOT NULL,
    request_hash BLOB NOT NULL CHECK(length(request_hash) = 32),
    action_type TEXT NOT NULL,
    pre_revision INTEGER NOT NULL CHECK(pre_revision >= 0),
    post_revision INTEGER NOT NULL CHECK(post_revision >= pre_revision),
    state_changed INTEGER NOT NULL CHECK(state_changed IN (0, 1)),
    result_json TEXT NOT NULL CHECK(json_valid(result_json)),
    result_category TEXT NOT NULL,
    random_metadata_json TEXT NOT NULL CHECK(json_valid(random_metadata_json)),
    created_at TEXT NOT NULL,
    UNIQUE(session_id, idempotency_key),
    CHECK((state_changed = 0 AND post_revision = pre_revision) OR
          (state_changed = 1 AND post_revision = pre_revision + 1))
);

CREATE INDEX action_results_session_created_idx ON action_results(session_id, created_at);

CREATE TABLE milestones (
    milestone_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL REFERENCES game_sessions(session_id) ON DELETE CASCADE,
    milestone_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    state_revision INTEGER NOT NULL CHECK(state_revision >= 0),
    announcement_allowed INTEGER NOT NULL CHECK(announcement_allowed IN (0, 1)),
    announced_at TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(session_id, milestone_key)
);
