package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestChannelCommandPrefixCRUD(t *testing.T) {
	db, cleanup := NewTestDB(t)
	defer cleanup()

	channel := "#prefix"

	if err := db.SetChannelState(channel, false); err != nil {
		t.Fatalf("SetChannelState failed: %v", err)
	}

	if err := db.SetChannelCommandPrefix(channel, "-"); err != nil {
		t.Fatalf("SetChannelCommandPrefix failed: %v", err)
	}

	prefix, err := db.GetChannelCommandPrefix(channel)
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix failed: %v", err)
	}
	if prefix != "-" {
		t.Fatalf("expected prefix '-', got %q", prefix)
	}

	enabled, err := db.GetChannelState(channel)
	if err != nil {
		t.Fatalf("GetChannelState failed: %v", err)
	}
	if enabled {
		t.Fatal("expected channel enabled state to remain false after prefix update")
	}

	prefixes, err := db.ListChannelCommandPrefixes()
	if err != nil {
		t.Fatalf("ListChannelCommandPrefixes failed: %v", err)
	}
	if got := prefixes[channel]; got != "-" {
		t.Fatalf("expected listed prefix '-', got %q", got)
	}

	if err := db.ClearChannelCommandPrefix(channel); err != nil {
		t.Fatalf("ClearChannelCommandPrefix failed: %v", err)
	}

	prefix, err = db.GetChannelCommandPrefix(channel)
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix after clear failed: %v", err)
	}
	if prefix != "" {
		t.Fatalf("expected cleared prefix to be empty, got %q", prefix)
	}

	prefixes, err = db.ListChannelCommandPrefixes()
	if err != nil {
		t.Fatalf("ListChannelCommandPrefixes after clear failed: %v", err)
	}
	if len(prefixes) != 0 {
		t.Fatalf("expected no channel prefixes after clear, got %d", len(prefixes))
	}
}

func TestChannelCommandPrefixPersistsAcrossReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := db.SetChannelCommandPrefix("#persist", "-"); err != nil {
		t.Fatalf("SetChannelCommandPrefix failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopened, err := New(dbPath)
	if err != nil {
		t.Fatalf("New reopen failed: %v", err)
	}
	defer func() {
		_ = reopened.Close()
	}()

	prefix, err := reopened.GetChannelCommandPrefix("#persist")
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix after reopen failed: %v", err)
	}
	if prefix != "-" {
		t.Fatalf("expected persisted prefix '-', got %q", prefix)
	}
}

func TestChannelCommandPrefixMigrationFromVersionEight(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "legacy.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}

	for _, migration := range migrations {
		if migration.Version > 8 {
			break
		}
		if _, err := raw.Exec(migration.UpSQL); err != nil {
			t.Fatalf("applying migration %d failed: %v", migration.Version, err)
		}
		if _, err := raw.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, 0)", migration.Version); err != nil {
			t.Fatalf("recording migration %d failed: %v", migration.Version, err)
		}
	}

	if _, err := raw.Exec(`INSERT INTO channel_states (channel, enabled, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, "#legacy", 0); err != nil {
		t.Fatalf("insert legacy channel state failed: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close failed: %v", err)
	}

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New upgrade failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	enabled, err := db.GetChannelState("#legacy")
	if err != nil {
		t.Fatalf("GetChannelState failed: %v", err)
	}
	if enabled {
		t.Fatal("expected legacy channel enabled state to remain false after migration")
	}

	prefix, err := db.GetChannelCommandPrefix("#legacy")
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix failed: %v", err)
	}
	if prefix != "" {
		t.Fatalf("expected empty prefix for migrated legacy channel, got %q", prefix)
	}

	if err := db.SetChannelCommandPrefix("#legacy", "-"); err != nil {
		t.Fatalf("SetChannelCommandPrefix after migration failed: %v", err)
	}

	prefix, err = db.GetChannelCommandPrefix("#legacy")
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix after set failed: %v", err)
	}
	if prefix != "-" {
		t.Fatalf("expected migrated prefix '-', got %q", prefix)
	}
}
