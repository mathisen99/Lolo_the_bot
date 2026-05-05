-- Rollback migration 010: Remove IRC network scope from channel-scoped data.

DROP INDEX IF EXISTS idx_messages_network_channel_timestamp;
DROP INDEX IF EXISTS idx_usage_network_channel;
DROP INDEX IF EXISTS idx_usage_network_timestamp;

DROP INDEX IF EXISTS idx_messages_timestamp;
DROP INDEX IF EXISTS idx_messages_channel;
DROP INDEX IF EXISTS idx_messages_nick;
DROP INDEX IF EXISTS idx_messages_event_type;

CREATE TABLE messages_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    channel TEXT,
    nick TEXT NOT NULL,
    hostmask TEXT,
    content TEXT NOT NULL,
    is_bot BOOLEAN DEFAULT 0,
    event_type TEXT DEFAULT NULL
);

INSERT INTO messages_old (id, timestamp, channel, nick, hostmask, content, is_bot, event_type)
SELECT id, timestamp, channel, nick, hostmask, content, is_bot, event_type
FROM messages
WHERE network = 'libera';

DROP TABLE messages;
ALTER TABLE messages_old RENAME TO messages;

CREATE INDEX idx_messages_timestamp ON messages(timestamp);
CREATE INDEX idx_messages_channel ON messages(channel);
CREATE INDEX idx_messages_nick ON messages(nick);
CREATE INDEX IF NOT EXISTS idx_messages_event_type ON messages(event_type);

CREATE TRIGGER cleanup_old_messages
AFTER INSERT ON messages
BEGIN
    DELETE FROM messages
    WHERE timestamp < datetime('now', '-90 days');
END;

DROP INDEX IF EXISTS idx_usage_timestamp;
DROP INDEX IF EXISTS idx_usage_nick;
DROP INDEX IF EXISTS idx_usage_channel;

CREATE TABLE usage_tracking_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    request_id TEXT,
    nick TEXT NOT NULL,
    channel TEXT,
    model TEXT NOT NULL,
    input_tokens INTEGER DEFAULT 0,
    cached_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd REAL DEFAULT 0.0,
    tool_calls INTEGER DEFAULT 0,
    web_search_calls INTEGER DEFAULT 0,
    code_interpreter_calls INTEGER DEFAULT 0
);

INSERT INTO usage_tracking_old (
    id, timestamp, request_id, nick, channel, model, input_tokens, cached_tokens,
    output_tokens, cost_usd, tool_calls, web_search_calls, code_interpreter_calls
)
SELECT id, timestamp, request_id, nick, channel, model, input_tokens, cached_tokens,
       output_tokens, cost_usd, tool_calls, web_search_calls, code_interpreter_calls
FROM usage_tracking
WHERE network = 'libera';

DROP TABLE usage_tracking;
ALTER TABLE usage_tracking_old RENAME TO usage_tracking;

CREATE INDEX idx_usage_timestamp ON usage_tracking(timestamp);
CREATE INDEX idx_usage_nick ON usage_tracking(nick);
CREATE INDEX idx_usage_channel ON usage_tracking(channel);

CREATE TABLE channel_states_old (
    channel TEXT PRIMARY KEY,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    command_prefix TEXT DEFAULT NULL
);

INSERT INTO channel_states_old (channel, enabled, created_at, updated_at, command_prefix)
SELECT channel, enabled, created_at, updated_at, command_prefix
FROM channel_states
WHERE network = 'libera';

DROP TABLE channel_states;
ALTER TABLE channel_states_old RENAME TO channel_states;

DROP INDEX IF EXISTS idx_channel_users_network_channel;
DROP INDEX IF EXISTS idx_channel_users_network_nick;

CREATE TABLE channel_users_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    nick TEXT NOT NULL,
    is_op BOOLEAN DEFAULT FALSE,
    is_halfop BOOLEAN DEFAULT FALSE,
    is_voice BOOLEAN DEFAULT FALSE,
    hostmask TEXT,
    account TEXT,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(channel, nick)
);

INSERT INTO channel_users_old (
    id, channel, nick, is_op, is_halfop, is_voice, hostmask, account, joined_at, updated_at
)
SELECT id, channel, nick, is_op, is_halfop, is_voice, hostmask, account, joined_at, updated_at
FROM channel_users
WHERE network = 'libera';

DROP TABLE channel_users;
ALTER TABLE channel_users_old RENAME TO channel_users;

CREATE INDEX IF NOT EXISTS idx_channel_users_channel ON channel_users(channel);
CREATE INDEX IF NOT EXISTS idx_channel_users_nick ON channel_users(nick);

CREATE TABLE bot_channel_status_old (
    channel TEXT PRIMARY KEY,
    is_joined BOOLEAN DEFAULT FALSE,
    is_op BOOLEAN DEFAULT FALSE,
    is_halfop BOOLEAN DEFAULT FALSE,
    is_voice BOOLEAN DEFAULT FALSE,
    user_count INTEGER DEFAULT 0,
    op_count INTEGER DEFAULT 0,
    voice_count INTEGER DEFAULT 0,
    topic TEXT,
    joined_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_channel_status_old (
    channel, is_joined, is_op, is_halfop, is_voice, user_count, op_count, voice_count, topic, joined_at, updated_at
)
SELECT channel, is_joined, is_op, is_halfop, is_voice, user_count, op_count, voice_count, topic, joined_at, updated_at
FROM bot_channel_status
WHERE network = 'libera';

DROP TABLE bot_channel_status;
ALTER TABLE bot_channel_status_old RENAME TO bot_channel_status;
