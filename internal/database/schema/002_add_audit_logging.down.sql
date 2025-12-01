-- Rollback audit logging table

DROP INDEX IF EXISTS idx_audit_target;
DROP INDEX IF EXISTS idx_audit_action;
DROP INDEX IF EXISTS idx_audit_actor;
DROP INDEX IF EXISTS idx_audit_timestamp;
DROP TABLE IF EXISTS audit_log;
