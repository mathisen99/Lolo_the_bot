-- Migration 007: Add channel user tracking
-- Tracks users in channels and their modes (op, voice, etc.)

-- Channel users table - tracks who is in each channel
CREATE TABLE IF NOT EXISTS channel_users (
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

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_channel_users_channel ON channel_users(channel);
CREATE INDEX IF NOT EXISTS idx_channel_users_nick ON channel_users(nick);

-- Bot channel status - tracks bot's own status in channels
CREATE TABLE IF NOT EXISTS bot_channel_status (
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
