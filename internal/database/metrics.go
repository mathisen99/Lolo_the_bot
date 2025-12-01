package database

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

// RecordCommandUsage records that a command was executed
// Requirement 30.1: Track command usage counts
func (db *DB) RecordCommandUsage(commandName string) error {
	_, err := db.conn.Exec(
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
// Requirement 30.2: Track API latency
func (db *DB) RecordAPILatency(latencyMs float64) error {
	_, err := db.conn.Exec(
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
// Requirement 30.3: Track error counts by type
func (db *DB) RecordError(errorType string) error {
	_, err := db.conn.Exec(
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
// Requirement 30.4, 30.5: Return statistics including uptime, command counts, average API latency, error counts
// and rolling statistics for 24h, 7d, 30d windows
func (db *DB) GetMetricsStats(startTime time.Time) (*MetricsStats, error) {
	stats := &MetricsStats{
		CommandCounts: make(map[string]int64),
		ErrorCounts:   make(map[string]int64),
		Uptime:        time.Since(startTime),
	}

	// Get command counts (all time)
	rows, err := db.conn.Query(
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
	err = db.conn.QueryRow(
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
	rows, err = db.conn.Query(
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
	stats.Stats24h, err = db.getTimeWindowStats(24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 24h stats: %w", err)
	}

	// Get 7-day stats
	stats.Stats7d, err = db.getTimeWindowStats(7 * 24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 7d stats: %w", err)
	}

	// Get 30-day stats
	stats.Stats30d, err = db.getTimeWindowStats(30 * 24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get 30d stats: %w", err)
	}

	return stats, nil
}

// getTimeWindowStats retrieves statistics for a specific time window
func (db *DB) getTimeWindowStats(window time.Duration) (*TimeWindowStats, error) {
	stats := &TimeWindowStats{}

	// Calculate cutoff time
	cutoffTime := time.Now().Add(-window)

	// Get command count in window
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeCommand,
		cutoffTime,
	).Scan(&stats.CommandCount)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query command count: %w", err)
	}

	// Get average latency in window
	var avgLatency sql.NullFloat64
	err = db.conn.QueryRow(
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
	err = db.conn.QueryRow(
		"SELECT COUNT(*) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeError,
		cutoffTime,
	).Scan(&stats.ErrorCount)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query error count: %w", err)
	}

	// Get unique command count in window
	err = db.conn.QueryRow(
		"SELECT COUNT(DISTINCT metric_name) FROM metrics WHERE metric_type = ? AND timestamp > ?",
		MetricTypeCommand,
		cutoffTime,
	).Scan(&stats.UniqueCommands)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query unique commands: %w", err)
	}

	// Get unique error types in window
	err = db.conn.QueryRow(
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
func (db *DB) CleanupOldMetrics(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	result, err := db.conn.Exec(
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
