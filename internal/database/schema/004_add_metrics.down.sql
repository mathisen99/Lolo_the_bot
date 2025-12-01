-- Rollback metrics table

DROP INDEX IF EXISTS idx_metrics_type_timestamp;
DROP INDEX IF EXISTS idx_metrics_type_name;
DROP INDEX IF EXISTS idx_metrics_timestamp;
DROP TABLE IF EXISTS metrics;

