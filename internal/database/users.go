package database

import (
	"database/sql"
	"fmt"
	"time"
)

// PermissionLevel represents user permission levels
type PermissionLevel int

const (
	LevelIgnored PermissionLevel = iota
	LevelNormal
	LevelAdmin
	LevelOwner
)

// User represents a bot user with permissions
type User struct {
	ID        int64
	Nick      string
	Hostmask  string
	Level     PermissionLevel
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateUser creates a new user in the database
func (db *DB) CreateUser(user *User) error {
	query := `
		INSERT INTO users (nick, hostmask, level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := db.conn.Exec(query, user.Nick, user.Hostmask, user.Level, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get user ID: %w", err)
	}

	user.ID = id
	return nil
}

// GetUser retrieves a user by nickname
func (db *DB) GetUser(nick string) (*User, error) {
	query := `
		SELECT id, nick, hostmask, level, created_at, updated_at
		FROM users
		WHERE nick = ?
	`
	user := &User{}
	err := db.conn.QueryRow(query, nick).Scan(
		&user.ID,
		&user.Nick,
		&user.Hostmask,
		&user.Level,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// GetUserByHostmask retrieves a user by hostmask
func (db *DB) GetUserByHostmask(hostmask string) (*User, error) {
	query := `
		SELECT id, nick, hostmask, level, created_at, updated_at
		FROM users
		WHERE hostmask = ?
	`
	user := &User{}
	err := db.conn.QueryRow(query, hostmask).Scan(
		&user.ID,
		&user.Nick,
		&user.Hostmask,
		&user.Level,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by hostmask: %w", err)
	}
	return user, nil
}

// UpdateUser updates an existing user
func (db *DB) UpdateUser(user *User) error {
	query := `
		UPDATE users
		SET nick = ?, hostmask = ?, level = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := db.conn.Exec(query, user.Nick, user.Hostmask, user.Level, time.Now(), user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// DeleteUser deletes a user by nickname
func (db *DB) DeleteUser(nick string) error {
	query := `DELETE FROM users WHERE nick = ?`
	_, err := db.conn.Exec(query, nick)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// ListUsers returns all users
func (db *DB) ListUsers() ([]*User, error) {
	query := `
		SELECT id, nick, hostmask, level, created_at, updated_at
		FROM users
		ORDER BY level DESC, nick ASC
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(
			&user.ID,
			&user.Nick,
			&user.Hostmask,
			&user.Level,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// HasOwner checks if an owner exists in the database
func (db *DB) HasOwner() (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM users WHERE level = ?", LevelOwner).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check for owner: %w", err)
	}
	return count > 0, nil
}
