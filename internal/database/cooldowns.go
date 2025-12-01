package database

import (
	"fmt"
	"time"
)

// CommandCooldown represents a user's last command usage
type CommandCooldown struct {
	UserNick    string
	CommandName string
	LastUsed    time.Time
}

// SetCommandCooldown records the last time a user used a command
func (db *DB) SetCommandCooldown(userNick, commandName string) error {
	query := `
		INSERT INTO command_cooldowns (user_nick, command_name, last_used)
		VALUES (?, ?, ?)
		ON CONFLICT(user_nick, command_name) DO UPDATE SET
			last_used = excluded.last_used
	`
	_, err := db.conn.Exec(query, userNick, commandName, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set command cooldown: %w", err)
	}
	return nil
}

// GetCommandCooldown retrieves the last time a user used a command
// Returns nil if no cooldown record exists
func (db *DB) GetCommandCooldown(userNick, commandName string) (*CommandCooldown, error) {
	query := `
		SELECT user_nick, command_name, last_used
		FROM command_cooldowns
		WHERE user_nick = ? AND command_name = ?
	`
	cooldown := &CommandCooldown{}
	err := db.conn.QueryRow(query, userNick, commandName).Scan(
		&cooldown.UserNick,
		&cooldown.CommandName,
		&cooldown.LastUsed,
	)
	if err != nil {
		// No rows found is not an error
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get command cooldown: %w", err)
	}
	return cooldown, nil
}

// CheckCommandCooldown checks if a user is on cooldown for a command
// Returns the remaining cooldown duration if on cooldown, or 0 if not on cooldown
func (db *DB) CheckCommandCooldown(userNick, commandName string, cooldownDuration time.Duration) time.Duration {
	cooldown, err := db.GetCommandCooldown(userNick, commandName)
	if err != nil {
		// If there's an error, assume no cooldown
		return 0
	}

	// If no cooldown record exists, user is not on cooldown
	if cooldown == nil {
		return 0
	}

	// Calculate remaining cooldown time
	elapsed := time.Since(cooldown.LastUsed)
	if elapsed >= cooldownDuration {
		// Cooldown has expired
		return 0
	}

	// Return remaining cooldown time
	return cooldownDuration - elapsed
}

// ClearCommandCooldown removes a cooldown record for a user and command
func (db *DB) ClearCommandCooldown(userNick, commandName string) error {
	query := `
		DELETE FROM command_cooldowns
		WHERE user_nick = ? AND command_name = ?
	`
	_, err := db.conn.Exec(query, userNick, commandName)
	if err != nil {
		return fmt.Errorf("failed to clear command cooldown: %w", err)
	}
	return nil
}

// ClearUserCooldowns removes all cooldown records for a user
func (db *DB) ClearUserCooldowns(userNick string) error {
	query := `DELETE FROM command_cooldowns WHERE user_nick = ?`
	_, err := db.conn.Exec(query, userNick)
	if err != nil {
		return fmt.Errorf("failed to clear user cooldowns: %w", err)
	}
	return nil
}

// ClearExpiredCooldowns removes all cooldown records older than the specified duration
// This can be used for periodic cleanup
func (db *DB) ClearExpiredCooldowns(maxAge time.Duration) error {
	query := `
		DELETE FROM command_cooldowns
		WHERE last_used < datetime('now', ?)
	`
	// Convert duration to SQLite format (negative seconds)
	seconds := -int64(maxAge.Seconds())
	_, err := db.conn.Exec(query, fmt.Sprintf("-%d seconds", seconds))
	if err != nil {
		return fmt.Errorf("failed to clear expired cooldowns: %w", err)
	}
	return nil
}
