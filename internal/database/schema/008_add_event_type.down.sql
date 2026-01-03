-- Rollback migration 008: Remove event_type from messages table

-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- This is a destructive operation that will lose event_type data

DROP INDEX IF EXISTS idx_messages_event_type;

-- Create new table without event_type
CREATE TABLE messages_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    channel TEXT,
    nick TEXT NOT NULL,
    hostmask TEXT,
    content TEXT NOT NULL,
    is_bot BOOLEAN DEFAULT 0
);

-- Copy data (excluding event_type)
INSERT INTO messages_new (id, timestamp, channel, nick, hostmask, content, is_bot)
SELECT id, timestamp, channel, nick, hostmask, content, is_bot FROM messages;

-- Drop old table and rename new one
DROP TABLE messages;
ALTER TABLE messages_new RENAME TO messages;

-- Recreate indexes
CREATE INDEX idx_messages_timestamp ON messages(timestamp);
CREATE INDEX idx_messages_channel ON messages(channel);
CREATE INDEX idx_messages_nick ON messages(nick);
