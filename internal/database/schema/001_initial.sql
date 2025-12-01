-- Initial database schema for Lolo IRC Bot

-- Users table: stores user information and permission levels
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nick TEXT NOT NULL UNIQUE,
    hostmask TEXT NOT NULL,
    level INTEGER NOT NULL,  -- 0=ignored, 1=normal, 2=admin, 3=owner
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_nick ON users(nick);
CREATE INDEX idx_users_hostmask ON users(hostmask);

-- Ensure only one owner can exist
CREATE TRIGGER enforce_single_owner
BEFORE INSERT ON users
WHEN NEW.level = 3 AND (SELECT COUNT(*) FROM users WHERE level = 3) > 0
BEGIN
    SELECT RAISE(ABORT, 'Only one owner can exist');
END;

CREATE TRIGGER enforce_single_owner_update
BEFORE UPDATE ON users
WHEN NEW.level = 3 AND OLD.level != 3 AND (SELECT COUNT(*) FROM users WHERE level = 3) > 0
BEGIN
    SELECT RAISE(ABORT, 'Only one owner can exist');
END;

-- Messages table: stores all IRC messages for logging and analysis
CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    channel TEXT,  -- Empty for PMs
    nick TEXT NOT NULL,
    hostmask TEXT,
    content TEXT NOT NULL,
    is_bot BOOLEAN DEFAULT 0
);

CREATE INDEX idx_messages_timestamp ON messages(timestamp);
CREATE INDEX idx_messages_channel ON messages(channel);
CREATE INDEX idx_messages_nick ON messages(nick);

-- Trigger to auto-delete messages older than retention period (90 days default)
CREATE TRIGGER cleanup_old_messages
AFTER INSERT ON messages
BEGIN
    DELETE FROM messages 
    WHERE timestamp < datetime('now', '-90 days');
END;

-- Channel states table: tracks enabled/disabled state for channels
CREATE TABLE channel_states (
    channel TEXT PRIMARY KEY,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Bot settings table: stores key-value configuration
CREATE TABLE bot_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Schema migrations tracking (used by golang-migrate)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);
