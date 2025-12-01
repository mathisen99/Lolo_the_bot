-- Add audit logging table for security-sensitive operations

-- Audit log table: tracks all administrative actions
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    actor_nick TEXT NOT NULL,
    actor_hostmask TEXT NOT NULL,
    action_type TEXT NOT NULL,  -- e.g., "user_add", "user_remove", "channel_enable", "pm_disable"
    target_user TEXT,  -- User affected by the action (nullable for non-user actions)
    details TEXT,  -- Additional context about the action
    result TEXT NOT NULL  -- "success" or "error"
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_actor ON audit_log(actor_nick);
CREATE INDEX idx_audit_action ON audit_log(action_type);
CREATE INDEX idx_audit_target ON audit_log(target_user);
