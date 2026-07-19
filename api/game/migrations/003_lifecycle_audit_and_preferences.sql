CREATE TABLE reset_tokens (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK(length(token_hash) = 32),
    session_id TEXT NOT NULL REFERENCES game_sessions(session_id) ON DELETE CASCADE,
    identity_key_hash BLOB NOT NULL CHECK(length(identity_key_hash) = 32),
    issued_revision INTEGER NOT NULL CHECK(issued_revision >= 0),
    expires_at TEXT NOT NULL,
    used_at TEXT,
    request_id TEXT NOT NULL UNIQUE
);

CREATE TABLE game_audits (
    audit_id TEXT PRIMARY KEY NOT NULL,
    request_id TEXT NOT NULL,
    session_ref_hash BLOB NOT NULL CHECK(length(session_ref_hash) = 32),
    event_type TEXT NOT NULL,
    pre_revision INTEGER CHECK(pre_revision IS NULL OR pre_revision >= 0),
    post_revision INTEGER CHECK(post_revision IS NULL OR post_revision >= 0),
    result_category TEXT NOT NULL,
    details_json TEXT NOT NULL CHECK(json_valid(details_json)),
    created_at TEXT NOT NULL
);

CREATE INDEX game_audits_request_idx ON game_audits(request_id);

CREATE TABLE content_preferences (
    session_id TEXT PRIMARY KEY NOT NULL REFERENCES game_sessions(session_id) ON DELETE CASCADE,
    selected_profile TEXT NOT NULL,
    adult_opt_in INTEGER NOT NULL DEFAULT 0 CHECK(adult_opt_in IN (0, 1)),
    milestone_opt_in INTEGER NOT NULL DEFAULT 0 CHECK(milestone_opt_in IN (0, 1)),
    category_restrictions_json TEXT NOT NULL CHECK(json_valid(category_restrictions_json)),
    policy_revision INTEGER NOT NULL CHECK(policy_revision >= 1),
    updated_at TEXT NOT NULL
);

CREATE TABLE identity_links (
    link_id TEXT PRIMARY KEY NOT NULL,
    network_id TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK(source_kind IN ('registered_user', 'unregistered_nick')),
    source_value TEXT NOT NULL,
    target_kind TEXT NOT NULL CHECK(target_kind IN ('registered_user', 'unregistered_nick')),
    target_value TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'resolved', 'rejected')),
    request_id TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    UNIQUE(network_id, source_kind, source_value, status)
);

CREATE TABLE session_archives (
    archive_id TEXT PRIMARY KEY NOT NULL,
    session_id TEXT NOT NULL,
    prior_revision INTEGER NOT NULL CHECK(prior_revision >= 0),
    state_ciphertext_or_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE maintenance_runs (
    run_id TEXT PRIMARY KEY NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('running', 'completed', 'failed')),
    started_at TEXT NOT NULL,
    finished_at TEXT,
    details_json TEXT NOT NULL CHECK(json_valid(details_json))
);

CREATE TABLE aggregate_metrics (
    metric_date TEXT NOT NULL,
    network_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value INTEGER NOT NULL CHECK(value >= 0),
    PRIMARY KEY(metric_date, network_id, metric_name)
);
