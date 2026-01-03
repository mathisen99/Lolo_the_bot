-- Rollback migration 007: Remove channel user tracking

DROP INDEX IF EXISTS idx_channel_users_channel;
DROP INDEX IF EXISTS idx_channel_users_nick;
DROP TABLE IF EXISTS channel_users;
DROP TABLE IF EXISTS bot_channel_status;
