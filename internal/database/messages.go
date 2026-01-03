package database

import (
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// Event types for IRC events logged to messages table
const (
	EventTypeMessage    = ""       // Regular message (default)
	EventTypeJoin       = "JOIN"   // User joined channel
	EventTypePart       = "PART"   // User left channel
	EventTypeQuit       = "QUIT"   // User quit IRC
	EventTypeKick       = "KICK"   // User was kicked
	EventTypeNickChange = "NICK"   // User changed nick
	EventTypeMode       = "MODE"   // Mode change (+o, +v, etc.)
	EventTypeTopic      = "TOPIC"  // Topic changed
	EventTypeBan        = "BAN"    // User was banned
	EventTypeUnban      = "UNBAN"  // User was unbanned
	EventTypeAction     = "ACTION" // /me action
)

// Message represents an IRC message or event
type Message struct {
	ID        int64
	Timestamp time.Time
	Channel   string // Empty for PMs or global events (QUIT)
	Nick      string
	Hostmask  string
	Content   string
	IsBot     bool
	EventType string // Empty for regular messages, otherwise event type
}

// LogMessage stores a message in the database
func (db *DB) LogMessage(msg *Message) error {
	query := `
		INSERT INTO messages (timestamp, channel, nick, hostmask, content, is_bot, event_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := db.conn.Exec(query, msg.Timestamp, msg.Channel, msg.Nick, msg.Hostmask, msg.Content, msg.IsBot, msg.EventType)
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

// LogEvent logs an IRC event (join, part, kick, quit, nick change, etc.)
func (db *DB) LogEvent(eventType, channel, nick, hostmask, content string) error {
	msg := &Message{
		Timestamp: time.Now(),
		Channel:   channel,
		Nick:      nick,
		Hostmask:  hostmask,
		Content:   content,
		IsBot:     false,
		EventType: eventType,
	}
	return db.LogMessage(msg)
}

// GetLastSeen retrieves the last message from a user (case-insensitive)
func (db *DB) GetLastSeen(nick string) (*Message, error) {
	query := `
		SELECT id, timestamp, channel, nick, hostmask, content, is_bot, COALESCE(event_type, '')
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
		&msg.EventType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get last seen: %w", err)
	}
	return msg, nil
}

// MessageFilter represents filters for querying messages
type MessageFilter struct {
	Channel       string
	Nick          string
	StartTime     time.Time
	EndTime       time.Time
	Limit         int
	EventType     string // Filter by event type (empty = all, "message" = only messages)
	IncludeEvents bool   // If true, include events; if false, only regular messages
}

// QueryMessages retrieves messages based on filters
func (db *DB) QueryMessages(filter *MessageFilter) ([]*Message, error) {
	query := `
		SELECT id, timestamp, channel, nick, hostmask, content, is_bot, COALESCE(event_type, '')
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

	// Filter by event type
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	} else if !filter.IncludeEvents {
		// By default, only return regular messages (no events)
		query += " AND (event_type IS NULL OR event_type = '')"
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
			&msg.EventType,
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
