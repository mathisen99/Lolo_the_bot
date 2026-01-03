-- Migration 008: Add event_type to messages table
-- Allows logging IRC events (kicks, bans, quits, nick changes, etc.)

-- Add event_type column to messages table
-- NULL = regular message, otherwise the event type
ALTER TABLE messages ADD COLUMN event_type TEXT DEFAULT NULL;

-- Add index for event queries
CREATE INDEX IF NOT EXISTS idx_messages_event_type ON messages(event_type);
