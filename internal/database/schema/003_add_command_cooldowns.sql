-- Add command cooldowns table for per-user command rate limiting

-- Command cooldowns table: tracks last command usage per user
CREATE TABLE command_cooldowns (
    user_nick TEXT NOT NULL,
    command_name TEXT NOT NULL,
    last_used DATETIME NOT NULL,
    PRIMARY KEY (user_nick, command_name)
);

CREATE INDEX idx_cooldowns_nick ON command_cooldowns(user_nick);
CREATE INDEX idx_cooldowns_command ON command_cooldowns(command_name);
