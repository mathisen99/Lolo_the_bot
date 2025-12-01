-- Add metrics table for tracking bot usage and performance

-- Metrics table: stores command usage, API latency, and error counts
CREATE TABLE metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    metric_type TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL
);

CREATE INDEX idx_metrics_timestamp ON metrics(timestamp);
CREATE INDEX idx_metrics_type_name ON metrics(metric_type, metric_name);
CREATE INDEX idx_metrics_type_timestamp ON metrics(metric_type, timestamp);

