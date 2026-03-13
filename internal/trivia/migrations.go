package trivia

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

type migration struct {
	Version int
	UpSQL   string
}

func (s *Store) runMigrations() error {
	if err := s.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	currentVersion, err := s.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}
		if err := s.applyMigration(m); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", m.Version, err)
		}
	}

	return nil
}

func (s *Store) ensureMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT 0
		)
	`
	_, err := s.conn.Exec(query)
	return err
}

func (s *Store) getCurrentVersion() (int, error) {
	var version int
	err := s.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = 0").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) applyMigration(m migration) error {
	tx, err := s.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, 1)", m.Version); err != nil {
		return fmt.Errorf("failed to mark migration dirty: %w", err)
	}

	if _, err := tx.Exec(m.UpSQL); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	if _, err := tx.Exec("UPDATE schema_migrations SET dirty = 0 WHERE version = ?", m.Version); err != nil {
		return fmt.Errorf("failed to mark migration clean: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "schema")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".sql") {
			continue
		}

		parts := strings.SplitN(filename, "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		content, err := fs.ReadFile(migrationFiles, "schema/"+filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		migrations = append(migrations, migration{
			Version: version,
			UpSQL:   string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
