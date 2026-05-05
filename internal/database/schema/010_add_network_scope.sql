-- Migration 010: Add IRC network scope to channel-scoped data.

ALTER TABLE messages ADD COLUMN network TEXT NOT NULL DEFAULT 'libera';
CREATE INDEX IF NOT EXISTS idx_messages_network_channel_timestamp
    ON messages(network, channel, timestamp DESC);

ALTER TABLE usage_tracking ADD COLUMN network TEXT NOT NULL DEFAULT 'libera';
CREATE INDEX IF NOT EXISTS idx_usage_network_channel
    ON usage_tracking(network, channel);
CREATE INDEX IF NOT EXISTS idx_usage_network_timestamp
    ON usage_tracking(network, timestamp);

CREATE TABLE channel_states_new (
    network TEXT NOT NULL DEFAULT 'libera',
    channel TEXT NOT NULL,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    command_prefix TEXT DEFAULT NULL,
    PRIMARY KEY(network, channel)
);

INSERT INTO channel_states_new (network, channel, enabled, created_at, updated_at, command_prefix)
SELECT 'libera', channel, enabled, created_at, updated_at, command_prefix
FROM channel_states;

DROP TABLE channel_states;
ALTER TABLE channel_states_new RENAME TO channel_states;

DROP INDEX IF EXISTS idx_channel_users_channel;
DROP INDEX IF EXISTS idx_channel_users_nick;

CREATE TABLE channel_users_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network TEXT NOT NULL DEFAULT 'libera',
    channel TEXT NOT NULL,
    nick TEXT NOT NULL,
    is_op BOOLEAN DEFAULT FALSE,
    is_halfop BOOLEAN DEFAULT FALSE,
    is_voice BOOLEAN DEFAULT FALSE,
    hostmask TEXT,
    account TEXT,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(network, channel, nick)
);

INSERT INTO channel_users_new (
    id, network, channel, nick, is_op, is_halfop, is_voice, hostmask, account, joined_at, updated_at
)
SELECT id, 'libera', channel, nick, is_op, is_halfop, is_voice, hostmask, account, joined_at, updated_at
FROM channel_users;

DROP TABLE channel_users;
ALTER TABLE channel_users_new RENAME TO channel_users;

CREATE INDEX IF NOT EXISTS idx_channel_users_network_channel
    ON channel_users(network, channel);
CREATE INDEX IF NOT EXISTS idx_channel_users_network_nick
    ON channel_users(network, nick);

CREATE TABLE bot_channel_status_new (
    network TEXT NOT NULL DEFAULT 'libera',
    channel TEXT NOT NULL,
    is_joined BOOLEAN DEFAULT FALSE,
    is_op BOOLEAN DEFAULT FALSE,
    is_halfop BOOLEAN DEFAULT FALSE,
    is_voice BOOLEAN DEFAULT FALSE,
    user_count INTEGER DEFAULT 0,
    op_count INTEGER DEFAULT 0,
    voice_count INTEGER DEFAULT 0,
    topic TEXT,
    joined_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(network, channel)
);

INSERT INTO bot_channel_status_new (
    network, channel, is_joined, is_op, is_halfop, is_voice, user_count, op_count, voice_count, topic, joined_at, updated_at
)
SELECT 'libera', channel, is_joined, is_op, is_halfop, is_voice, user_count, op_count, voice_count, topic, joined_at, updated_at
FROM bot_channel_status;

DROP TABLE bot_channel_status;
ALTER TABLE bot_channel_status_new RENAME TO bot_channel_status;
