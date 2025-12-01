package database

import (
	"fmt"
	"time"
)

// AuditLogEntry represents an entry in the audit log
type AuditLogEntry struct {
	ID            int64
	Timestamp     time.Time
	ActorNick     string
	ActorHostmask string
	ActionType    string
	TargetUser    *string // nullable
	Details       *string // nullable
	Result        string
}

// LogAuditAction logs a security-sensitive action to the audit log
// Requirement 29.1: Log user management actions
// Requirement 29.2: Log channel state changes
// Requirement 29.3: Log PM state changes
func (db *DB) LogAuditAction(actorNick, actorHostmask, actionType string, targetUser, details, result string) error {
	query := `
		INSERT INTO audit_log (actor_nick, actor_hostmask, action_type, target_user, details, result, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	var targetUserPtr *string
	if targetUser != "" {
		targetUserPtr = &targetUser
	}

	var detailsPtr *string
	if details != "" {
		detailsPtr = &details
	}

	_, err := db.conn.Exec(
		query,
		actorNick,
		actorHostmask,
		actionType,
		targetUserPtr,
		detailsPtr,
		result,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to log audit action: %w", err)
	}

	return nil
}

// GetAuditLog retrieves audit log entries with optional filtering
// Requirement 29.4: Support filtering by date range, actor, action type, and target
func (db *DB) GetAuditLog(limit int, offset int) ([]*AuditLogEntry, error) {
	query := `
		SELECT id, timestamp, actor_nick, actor_hostmask, action_type, target_user, details, result
		FROM audit_log
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*AuditLogEntry
	for rows.Next() {
		entry := &AuditLogEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.ActorNick,
			&entry.ActorHostmask,
			&entry.ActionType,
			&entry.TargetUser,
			&entry.Details,
			&entry.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log: %w", err)
	}

	return entries, nil
}

// GetAuditLogByActor retrieves audit log entries filtered by actor
func (db *DB) GetAuditLogByActor(actorNick string, limit int, offset int) ([]*AuditLogEntry, error) {
	query := `
		SELECT id, timestamp, actor_nick, actor_hostmask, action_type, target_user, details, result
		FROM audit_log
		WHERE actor_nick = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, actorNick, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by actor: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*AuditLogEntry
	for rows.Next() {
		entry := &AuditLogEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.ActorNick,
			&entry.ActorHostmask,
			&entry.ActionType,
			&entry.TargetUser,
			&entry.Details,
			&entry.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log: %w", err)
	}

	return entries, nil
}

// GetAuditLogByActionType retrieves audit log entries filtered by action type
func (db *DB) GetAuditLogByActionType(actionType string, limit int, offset int) ([]*AuditLogEntry, error) {
	query := `
		SELECT id, timestamp, actor_nick, actor_hostmask, action_type, target_user, details, result
		FROM audit_log
		WHERE action_type = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, actionType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by action type: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*AuditLogEntry
	for rows.Next() {
		entry := &AuditLogEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.ActorNick,
			&entry.ActorHostmask,
			&entry.ActionType,
			&entry.TargetUser,
			&entry.Details,
			&entry.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log: %w", err)
	}

	return entries, nil
}

// GetAuditLogByTarget retrieves audit log entries filtered by target user
func (db *DB) GetAuditLogByTarget(targetUser string, limit int, offset int) ([]*AuditLogEntry, error) {
	query := `
		SELECT id, timestamp, actor_nick, actor_hostmask, action_type, target_user, details, result
		FROM audit_log
		WHERE target_user = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, targetUser, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by target: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*AuditLogEntry
	for rows.Next() {
		entry := &AuditLogEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.ActorNick,
			&entry.ActorHostmask,
			&entry.ActionType,
			&entry.TargetUser,
			&entry.Details,
			&entry.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log: %w", err)
	}

	return entries, nil
}

// GetAuditLogByDateRange retrieves audit log entries within a date range
func (db *DB) GetAuditLogByDateRange(startTime, endTime time.Time, limit int, offset int) ([]*AuditLogEntry, error) {
	query := `
		SELECT id, timestamp, actor_nick, actor_hostmask, action_type, target_user, details, result
		FROM audit_log
		WHERE timestamp BETWEEN ? AND ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, startTime, endTime, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by date range: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*AuditLogEntry
	for rows.Next() {
		entry := &AuditLogEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.ActorNick,
			&entry.ActorHostmask,
			&entry.ActionType,
			&entry.TargetUser,
			&entry.Details,
			&entry.Result,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log: %w", err)
	}

	return entries, nil
}
