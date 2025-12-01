package database

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed schema/*.sql
var migrationFiles embed.FS

// Migration represents a database migration
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// runMigrations runs all pending database migrations
func (db *DB) runMigrations() error {
	// Ensure schema_migrations table exists
	if err := db.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Load all migrations from embedded files
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Get current version
	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Apply pending migrations
	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}

		if err := db.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist
func (db *DB) ensureMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT 0
		)
	`
	_, err := db.conn.Exec(query)
	return err
}

// getCurrentVersion returns the current migration version
func (db *DB) getCurrentVersion() (int, error) {
	var version int
	err := db.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = 0").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// applyMigration applies a single migration
func (db *DB) applyMigration(migration Migration) error {
	// Start transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Mark migration as dirty (in progress)
	_, err = tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, 1)", migration.Version)
	if err != nil {
		return fmt.Errorf("failed to mark migration as dirty: %w", err)
	}

	// Execute migration SQL
	_, err = tx.Exec(migration.UpSQL)
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Mark migration as clean (completed)
	_, err = tx.Exec("UPDATE schema_migrations SET dirty = 0 WHERE version = ?", migration.Version)
	if err != nil {
		return fmt.Errorf("failed to mark migration as clean: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

// loadMigrations loads all migration files from the embedded filesystem
func loadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "schema")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory: %w", err)
	}

	// Group migrations by version
	migrationMap := make(map[int]*Migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial.sql" -> 1)
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		// Read file content
		content, err := fs.ReadFile(migrationFiles, "schema/"+name)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		// Get or create migration entry
		if migrationMap[version] == nil {
			migrationMap[version] = &Migration{
				Version: version,
				Name:    strings.TrimSuffix(parts[1], ".sql"),
			}
		}

		// Store SQL based on file type
		if strings.HasSuffix(name, ".down.sql") {
			migrationMap[version].DownSQL = string(content)
		} else {
			migrationMap[version].UpSQL = string(content)
		}
	}

	// Convert map to sorted slice
	var migrations []Migration
	for _, migration := range migrationMap {
		if migration.UpSQL != "" { // Only include migrations with up SQL
			migrations = append(migrations, *migration)
		}
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// Rollback rolls back the last applied migration
func (db *DB) Rollback() error {
	// Get current version
	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// Load migrations
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Find the migration to rollback
	var targetMigration *Migration
	for _, m := range migrations {
		if m.Version == currentVersion {
			targetMigration = &m
			break
		}
	}

	if targetMigration == nil {
		return fmt.Errorf("migration %d not found", currentVersion)
	}

	if targetMigration.DownSQL == "" {
		return fmt.Errorf("migration %d has no down SQL", currentVersion)
	}

	// Start transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Execute down migration
	_, err = tx.Exec(targetMigration.DownSQL)
	if err != nil {
		return fmt.Errorf("failed to execute down migration: %w", err)
	}

	// Remove migration record
	_, err = tx.Exec("DELETE FROM schema_migrations WHERE version = ?", currentVersion)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	return nil
}
