package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection and provides access to database operations
type DB struct {
	conn *sql.DB
	path string
}

// New creates a new database connection
// If the database file doesn't exist, it will be created
func New(dbPath string) (*DB, error) {
	// Ensure the data directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Check if this is a first-time initialization
	isFirstRun := !fileExists(dbPath)

	// Open database connection
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn: conn,
		path: dbPath,
	}

	// Enable WAL mode for better concurrency (Requirement 12.6)
	if err := db.configureWAL(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to configure WAL mode: %w", err)
	}

	// Run migrations on first run or if needed
	if isFirstRun {
		if err := db.runMigrations(); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	return db, nil
}

// NewTest creates a new test database connection using an in-memory database
// This is useful for testing without affecting the production database
func NewTest() (*DB, error) {
	// Use in-memory SQLite database for testing
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Test the connection
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	db := &DB{
		conn: conn,
		path: ":memory:",
	}

	// Enable WAL mode for better concurrency (Requirement 12.6)
	if err := db.configureWAL(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to configure WAL mode: %w", err)
	}

	// Run migrations for test database
	if err := db.runMigrations(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to run migrations on test database: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn returns the underlying database connection
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// configureWAL enables Write-Ahead Logging mode and configures checkpoint settings
// for better concurrency and performance (Requirement 12.6)
func (db *DB) configureWAL() error {
	// Enable WAL mode
	var journalMode string
	err := db.conn.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode)
	if err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("failed to enable WAL mode: got %s instead", journalMode)
	}

	// Configure checkpoint settings for WAL mode
	// PRAGMA wal_autocheckpoint: number of pages before automatic checkpoint (default 1000)
	// Setting to 5000 reduces checkpoint frequency for better write performance
	_, err = db.conn.Exec("PRAGMA wal_autocheckpoint=5000")
	if err != nil {
		return fmt.Errorf("failed to configure WAL autocheckpoint: %w", err)
	}

	// PRAGMA synchronous: controls how strictly SQLite syncs to disk
	// NORMAL (1) is safe for WAL mode and provides good performance
	_, err = db.conn.Exec("PRAGMA synchronous=NORMAL")
	if err != nil {
		return fmt.Errorf("failed to configure synchronous mode: %w", err)
	}

	// PRAGMA busy_timeout: milliseconds to wait when database is locked
	// Setting to 5000ms (5 seconds) prevents "database is locked" errors during concurrent access
	_, err = db.conn.Exec("PRAGMA busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to configure busy timeout: %w", err)
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
