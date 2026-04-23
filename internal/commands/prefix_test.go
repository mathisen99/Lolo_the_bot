package commands

import (
	"strings"
	"testing"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

func TestPrefixCommandSetShowAndReset(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	dispatcher := NewDispatcher(NewRegistry(), user.NewManager(db), "!")
	cmd := NewPrefixCommand(db, dispatcher)

	showCtx := NewContext("prefix", []string{"show"}, "!prefix show", "admin", "admin@host", "#chan", false, database.LevelAdmin, true, "!")
	resp, err := cmd.Execute(showCtx)
	if err != nil {
		t.Fatalf("Execute show failed: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Message, "(default)") {
		t.Fatalf("expected default prefix message, got %#v", resp)
	}

	setCtx := NewContext("prefix", []string{"-"}, "!prefix -", "admin", "admin@host", "#chan", false, database.LevelAdmin, true, "!")
	resp, err = cmd.Execute(setCtx)
	if err != nil {
		t.Fatalf("Execute set failed: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Message, `Use -prefix !`) {
		t.Fatalf("expected reset hint in response, got %#v", resp)
	}

	active := dispatcher.GetActivePrefix("#chan", false)
	if active != "-" {
		t.Fatalf("expected dispatcher prefix '-', got %q", active)
	}

	stored, err := db.GetChannelCommandPrefix("#chan")
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix failed: %v", err)
	}
	if stored != "-" {
		t.Fatalf("expected stored prefix '-', got %q", stored)
	}

	entries, err := db.GetAuditLogByActionType("channel_prefix_set", 10, 0)
	if err != nil {
		t.Fatalf("GetAuditLogByActionType(channel_prefix_set) failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one audit entry for prefix set, got %d", len(entries))
	}

	showCustomCtx := NewContext("prefix", []string{"show"}, "-prefix show", "admin", "admin@host", "#chan", false, database.LevelAdmin, true, "-")
	resp, err = cmd.Execute(showCustomCtx)
	if err != nil {
		t.Fatalf("Execute show custom failed: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Message, "(custom)") {
		t.Fatalf("expected custom prefix message, got %#v", resp)
	}

	resetCtx := NewContext("prefix", []string{"!"}, "-prefix !", "admin", "admin@host", "#chan", false, database.LevelAdmin, true, "-")
	resp, err = cmd.Execute(resetCtx)
	if err != nil {
		t.Fatalf("Execute reset failed: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Message, `reset to "!"`) {
		t.Fatalf("expected reset response, got %#v", resp)
	}

	active = dispatcher.GetActivePrefix("#chan", false)
	if active != "!" {
		t.Fatalf("expected dispatcher prefix '!' after reset, got %q", active)
	}

	stored, err = db.GetChannelCommandPrefix("#chan")
	if err != nil {
		t.Fatalf("GetChannelCommandPrefix after reset failed: %v", err)
	}
	if stored != "" {
		t.Fatalf("expected cleared stored prefix, got %q", stored)
	}

	entries, err = db.GetAuditLogByActionType("channel_prefix_reset", 10, 0)
	if err != nil {
		t.Fatalf("GetAuditLogByActionType(channel_prefix_reset) failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one audit entry for prefix reset, got %d", len(entries))
	}
}

func TestPrefixCommandValidationAndPMRejection(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	dispatcher := NewDispatcher(NewRegistry(), user.NewManager(db), "!")
	cmd := NewPrefixCommand(db, dispatcher)

	pmCtx := NewContext("prefix", []string{"-"}, "!prefix -", "admin", "admin@host", "", true, database.LevelAdmin, true, "!")
	resp, err := cmd.Execute(pmCtx)
	if err != nil {
		t.Fatalf("Execute PM failed: %v", err)
	}
	if resp == nil || resp.Message != "Prefix is channel-specific. Use this command in a channel." {
		t.Fatalf("unexpected PM response: %#v", resp)
	}

	for _, invalid := range []string{"", "ab", "a", "1", " "} {
		ctx := NewContext("prefix", []string{invalid}, "!prefix "+invalid, "admin", "admin@host", "#chan", false, database.LevelAdmin, true, "!")
		resp, err := cmd.Execute(ctx)
		if err != nil {
			t.Fatalf("Execute invalid prefix %q returned error: %v", invalid, err)
		}
		if resp == nil || resp.Message != "Prefix must be a single symbol character, for example !, -, ., or ?" {
			t.Fatalf("unexpected invalid prefix response for %q: %#v", invalid, resp)
		}
	}
}

func TestPrefixCommandPermissionEnforcedByRegistry(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	registry := NewRegistry()
	dispatcher := NewDispatcher(registry, user.NewManager(db), "!")
	cmd := NewPrefixCommand(db, dispatcher)
	if err := registry.Register(cmd); err != nil {
		t.Fatalf("Register prefix command failed: %v", err)
	}

	ctx := NewContext("prefix", []string{"-"}, "!prefix -", "alice", "", "#chan", false, database.LevelNormal, false, "!")
	_, err := registry.Execute(ctx)
	if err == nil {
		t.Fatal("expected permission error for normal user")
	}
}
