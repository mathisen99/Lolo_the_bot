package database

import (
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// Message represents an IRC message
type Message struct {
	ID        int64
	Timestamp time.Time
	Channel   string // Empty for PMs
	Nick      string
	Hostmask  string
	Content   string
	IsBot     bool
}

// LogMessage stores a message in the database
func (db *DB) LogMessage(msg *Message) error {
	query := `
		INSERT INTO messages (timestamp, channel, nick, hostmask, content, is_bot)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := db.conn.Exec(query, msg.Timestamp, msg.Channel, msg.Nick, msg.Hostmask, msg.Content, msg.IsBot)
	if err != nil {
		return fmt.Errorf("failed to log message: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get message ID: %w", err)
	}

	msg.ID = id
	return nil
}

// GetLastSeen retrieves the last message from a user (case-insensitive)
func (db *DB) GetLastSeen(nick string) (*Message, error) {
	query := `
		SELECT id, timestamp, channel, nick, hostmask, content, is_bot
		FROM messages
		WHERE LOWER(nick) = LOWER(?)
		ORDER BY timestamp DESC
		LIMIT 1
	`
	msg := &Message{}
	err := db.conn.QueryRow(query, nick).Scan(
		&msg.ID,
		&msg.Timestamp,
		&msg.Channel,
		&msg.Nick,
		&msg.Hostmask,
		&msg.Content,
		&msg.IsBot,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get last seen: %w", err)
	}
	return msg, nil
}

// MessageFilter represents filters for querying messages
type MessageFilter struct {
	Channel   string
	Nick      string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// QueryMessages retrieves messages based on filters
func (db *DB) QueryMessages(filter *MessageFilter) ([]*Message, error) {
	query := `
		SELECT id, timestamp, channel, nick, hostmask, content, is_bot
		FROM messages
		WHERE 1=1
	`
	args := []interface{}{}

	if filter.Channel != "" {
		query += " AND channel = ?"
		args = append(args, filter.Channel)
	}

	if filter.Nick != "" {
		query += " AND nick = ?"
		args = append(args, filter.Nick)
	}

	if !filter.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.StartTime)
	}

	if !filter.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(
			&msg.ID,
			&msg.Timestamp,
			&msg.Channel,
			&msg.Nick,
			&msg.Hostmask,
			&msg.Content,
			&msg.IsBot,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// CleanupOldMessages deletes messages older than the specified retention days
// This implements Requirement 14.6 and 14.7 - automatic cleanup of old messages
// The logger parameter is used to log cleanup operations (Requirement 14.6)
func (db *DB) CleanupOldMessages(retentionDays int, logger output.Logger) error {
	if retentionDays <= 0 {
		return fmt.Errorf("retention days must be positive, got %d", retentionDays)
	}

	startTime := time.Now()
	logger.Info("Starting message cleanup (retention: %d days)...", retentionDays)

	// Count messages before cleanup
	countBefore := 0
	err := db.conn.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countBefore)
	if err != nil {
		return fmt.Errorf("failed to count messages before cleanup: %w", err)
	}

	// Delete messages older than retention period
	query := `
		DELETE FROM messages 
		WHERE timestamp < datetime('now', '-' || ? || ' days')
	`
	result, err := db.conn.Exec(query, retentionDays)
	if err != nil {
		logger.Error("Message cleanup failed: %v", err)
		return fmt.Errorf("failed to cleanup old messages: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Count messages after cleanup
	countAfter := 0
	err = db.conn.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countAfter)
	if err != nil {
		return fmt.Errorf("failed to count messages after cleanup: %w", err)
	}

	duration := time.Since(startTime)

	// Log cleanup operation (Requirement 14.6)
	logger.Success("Message cleanup completed in %.2f seconds: deleted %d messages (%d → %d remaining)",
		duration.Seconds(), rowsAffected, countBefore, countAfter)

	return nil
}
