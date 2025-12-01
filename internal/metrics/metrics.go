package metrics

import (
	"database/sql"
	"fmt"
	"time"
)

// MetricType represents the type of metric being recorded
type MetricType string

const (
	MetricTypeCommand    MetricType = "command"
	MetricTypeAPILatency MetricType = "api_latency"
	MetricTypeError      MetricType = "error"
)

// Metric represents a single metric data point
type Metric struct {
	ID        int64
	Timestamp time.Time
	Type      MetricType
	Name      string
	Value     float64
}

// MetricsStats holds aggregated metrics statistics
type MetricsStats struct {
	Uptime            time.Duration
	CommandCounts     map[string]int64
	AverageAPILatency float64
	ErrorCounts       map[string]int64
	Stats24h          *TimeWindowStats
	Stats7d           *TimeWindowStats
	Stats30d          *TimeWindowStats
}

// TimeWindowStats holds statistics for a specific time window
type TimeWindowStats struct {
	CommandCount     int64
	AverageLatency   float64
	ErrorCount       int64
	UniqueCommands   int64
	UniqueErrorTypes int64
}

// Collector provides metrics collection functionality
type Collector struct {
	conn *sql.DB
}

// NewCollector creates a new metrics collector
func NewCollector(conn *sql.DB) *Collector {
	return &Collector{conn: conn}
}

// RecordCommandUsage records that a command was executed
func (c *Collector) RecordCommandUsage(commandName string) error {
	_, err := c.conn.Exec(
		"INSERT INTO metrics (metric_type, metric_name, value) VALUES (?, ?, ?)",
		MetricTypeCommand,
		commandName,
		1.0,
	)
	if err != nil {
		return fmt.Errorf("failed to record command usage: %w", err)
	}
	return nil
}

// RecordAPILatency records the latency of an API request in milliseconds
func (c *Collector) RecordAPILatency(latencyMs float64) error {
	_, err := c.conn.Exec(
		"INSERT INTO metrics (metric_type, metric_name, value) VALUES (?, ?, ?)",
		MetricTypeAPILatency,
		"api_request",
		latencyMs,
	)
	if err != nil {
		return fmt.Errorf("failed to record API latency: %w", err)
	}
	return nil
}

// RecordError records that an error occurred
func (c *Collector) RecordError(errorType string) error {
	_, err := c.conn.Exec(
		"INSERT INTO metrics (metric_type, metric_name, value) VALUES (?, ?, ?)",
		MetricTypeError,
		errorType,
		1.0,
	)
	if err != nil {
		return fmt.Errorf("failed to record error: %w", err)
	}
	return nil
}

// GetMetricsStats retrieves aggregated metrics statistics
func (c *Collector) GetMetricsStats(startTime time.Time) (*MetricsStats, error) {
	stats := &MetricsStats{
		CommandCounts: make(map[string]int64),
		ErrorCounts:   make(map[string]int64),
		Uptime:        time.Since(startTime),
	}

	// Get command counts (all time)
	rows, err := c.conn.Query(
		"SELECT metric_name, COUNT(*) as count FROM metrics WHERE metric_type = ? GROUP BY metric_name",
		MetricTypeCommand,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query command counts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("failed to scan command count: %w", err)
		}
		stats.CommandCounts[name] = count
	}

	// Get average API latency (all time)
	var avgLatency sql.NullFloat64
	err = c.conn.QueryRow(
		"SELECT AVG(value) FROM metrics WHERE metric_type = ?",
		MetricTypeAPILatency,
	).Scan(&avgLatency)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query average API latency: %w", err)
	}
	if avgLatency.Valid {
		stats.AverageAPILatency = avgLatency.Float64
	}

	// Get error counts (all time)
	rows, err = c.conn.Query(
		"SELECT metric_name, COUNT(*) as count FROM metrics WHERE metric_type = ? GROUP BY metric_name",
		MetricTypeError,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query error counts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("failed to scan error count: %w", err)
		}
		stats.ErrorCounts[name] = count
	}

	// Get 24-hour stats
	stats.Stats24h, err = c.getTimeWindowStats(24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 24h stats: %w", err)
	}

	// Get 7-day stats
	stats.Stats7d, err = c.getTimeWindowStats(7 * 24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 7d stats: %w", err)
	}

	// Get 30-day stats
	stats.Stats30d, err = c.getTimeWindowStats(30 * 24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 30d stats: %w", err)
	}

	return stats, nil
}

// getTimeWindowStats retrieves statistics for a specific time window
func (c *Collector) getTimeWindowStats(window time.Duration) (*TimeWindowStats, error) {
	stats := &TimeWindowStats{}

	// Calculate cutoff time
	cutoffTime := time.Now().Add(-window)

	// Get command count in window
	err := c.conn.QueryRow(
		"SELECT COUNT(*) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeCommand,
		cutoffTime,
	).Scan(&stats.CommandCount)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query command count: %w", err)
	}

	// Get average latency in window
	var avgLatency sql.NullFloat64
	err = c.conn.QueryRow(
		"SELECT AVG(value) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeAPILatency,
		cutoffTime,
	).Scan(&avgLatency)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query average latency: %w", err)
	}
	if avgLatency.Valid {
		stats.AverageLatency = avgLatency.Float64
	}

	// Get error count in window
	err = c.conn.QueryRow(
		"SELECT COUNT(*) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeError,
		cutoffTime,
	).Scan(&stats.ErrorCount)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query error count: %w", err)
	}

	// Get unique command count in window
	err = c.conn.QueryRow(
		"SELECT COUNT(DISTINCT metric_name) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeCommand,
		cutoffTime,
	).Scan(&stats.UniqueCommands)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query unique commands: %w", err)
	}

	// Get unique error types in window
	err = c.conn.QueryRow(
		"SELECT COUNT(DISTINCT metric_name) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeError,
		cutoffTime,
	).Scan(&stats.UniqueErrorTypes)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query unique error types: %w", err)
	}

	return stats, nil
}

// CleanupOldMetrics deletes metrics older than the specified duration
func (c *Collector) CleanupOldMetrics(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	result, err := c.conn.Exec(
		"DELETE FROM metrics WHERE timestamp < ?",
		cutoffTime,
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup old metrics: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		// Log cleanup but don't fail if logging fails
		_ = fmt.Sprintf("Cleaned up %d old metrics", rowsAffected)
	}

	return nil
}
