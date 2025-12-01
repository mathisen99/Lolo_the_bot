-- Rollback initial schema

DROP TRIGGER IF EXISTS cleanup_old_messages;
DROP TRIGGER IF EXISTS enforce_single_owner_update;
DROP TRIGGER IF EXISTS enforce_single_owner;
DROP INDEX IF EXISTS idx_messages_nick;
DROP INDEX IF EXISTS idx_messages_channel;
DROP INDEX IF EXISTS idx_messages_timestamp;
DROP INDEX IF EXISTS idx_users_hostmask;
DROP INDEX IF EXISTS idx_users_nick;
DROP TABLE IF EXISTS bot_settings;
DROP TABLE IF EXISTS channel_states;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS users;
