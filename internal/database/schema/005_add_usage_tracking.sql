-- Usage tracking for AI API costs
-- Tracks token usage and costs per request for billing/analytics

CREATE TABLE usage_tracking (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    request_id TEXT,
    nick TEXT NOT NULL,
    channel TEXT,
    model TEXT NOT NULL,
    input_tokens INTEGER DEFAULT 0,
    cached_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd REAL DEFAULT 0.0,
    tool_calls INTEGER DEFAULT 0
);

CREATE INDEX idx_usage_timestamp ON usage_tracking(timestamp);
CREATE INDEX idx_usage_nick ON usage_tracking(nick);
CREATE INDEX idx_usage_channel ON usage_tracking(channel);
