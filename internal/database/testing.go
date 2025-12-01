package database

import (
	"path/filepath"
	"testing"
)

// NewTestDB creates a new test database and returns a cleanup function
// This is exported so other packages can use it for testing
func NewTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Return database and cleanup function
	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close test database: %v", err)
		}
	}

	return db, cleanup
}
