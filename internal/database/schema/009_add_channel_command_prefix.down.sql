-- Rollback migration 009: Remove per-channel command prefix overrides

-- SQLite doesn't support DROP COLUMN directly, so recreate the table.

CREATE TABLE channel_states_new (
    channel TEXT PRIMARY KEY,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO channel_states_new (channel, enabled, created_at, updated_at)
SELECT channel, enabled, created_at, updated_at
FROM channel_states;

DROP TABLE channel_states;
ALTER TABLE channel_states_new RENAME TO channel_states;
