package database

import "testing"

func TestGetUserMatchesIRCNickCaseInsensitively(t *testing.T) {
	db, cleanup := NewTestDB(t)
	defer cleanup()

	created := &User{Nick: "MixedCase", Hostmask: "ident@example", Level: LevelIgnored}
	if err := db.CreateUser(created); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	found, err := db.GetUser("mixedcase")
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetUser() returned nil for a differently-cased IRC nick")
	}
	if found.Level != LevelIgnored {
		t.Fatalf("GetUser() level = %v, want %v", found.Level, LevelIgnored)
	}
}
