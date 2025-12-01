-- Rollback command cooldowns table

DROP INDEX IF EXISTS idx_cooldowns_command;
DROP INDEX IF EXISTS idx_cooldowns_nick;
DROP TABLE IF EXISTS command_cooldowns;
