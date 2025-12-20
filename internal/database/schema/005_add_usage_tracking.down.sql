-- Rollback usage tracking

DROP INDEX IF EXISTS idx_usage_timestamp;
DROP INDEX IF EXISTS idx_usage_nick;
DROP INDEX IF EXISTS idx_usage_channel;
DROP TABLE IF EXISTS usage_tracking;
