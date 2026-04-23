package commands

import (
	"testing"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

type testCommand struct {
	name    string
	level   database.PermissionLevel
	help    string
	execute func(ctx *Context) (*Response, error)
}

func (c *testCommand) Name() string { return c.name }

func (c *testCommand) Execute(ctx *Context) (*Response, error) {
	if c.execute != nil {
		return c.execute(ctx)
	}
	return NewResponse(c.name), nil
}

func (c *testCommand) RequiredPermission() database.PermissionLevel { return c.level }
func (c *testCommand) Help() string                                 { return c.help }
func (c *testCommand) CooldownDuration() time.Duration              { return 0 }

func TestDispatcherResolvesChannelPrefixes(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	registry := NewRegistry()
	dispatcher := NewDispatcher(registry, user.NewManager(db), "!")

	if err := registry.Register(&testCommand{name: "help", level: database.LevelNormal}); err != nil {
		t.Fatalf("Register help failed: %v", err)
	}

	if !dispatcher.IsCommand("#chan", "!help", false) {
		t.Fatal("expected !help to be a command with default prefix")
	}

	command, args := dispatcher.ParseCommand("#chan", "!help me", false)
	if command != "help" {
		t.Fatalf("expected command help, got %q", command)
	}
	if len(args) != 1 || args[0] != "me" {
		t.Fatalf("expected args [me], got %v", args)
	}

	dispatcher.SetChannelPrefix("#chan", "-")

	if dispatcher.IsCommand("#chan", "!help", false) {
		t.Fatal("expected !help to stop working after prefix override")
	}
	if !dispatcher.IsCommand("#chan", "-help", false) {
		t.Fatal("expected -help to work after prefix override")
	}

	command, args = dispatcher.ParseCommand("#chan", "-help now", false)
	if command != "help" {
		t.Fatalf("expected overridden command help, got %q", command)
	}
	if len(args) != 1 || args[0] != "now" {
		t.Fatalf("expected args [now], got %v", args)
	}

	if !dispatcher.IsCommand("", "!help", true) {
		t.Fatal("expected PMs to continue using the default prefix")
	}
	if dispatcher.IsCommand("", "-help", true) {
		t.Fatal("expected PMs to ignore channel overrides")
	}

	dispatcher.ClearChannelPrefix("#chan")
	if !dispatcher.IsCommand("#chan", "!help", false) {
		t.Fatal("expected !help to work again after clearing the override")
	}
}

func TestDispatcherDispatchesActivePrefixInContext(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	registry := NewRegistry()
	dispatcher := NewDispatcher(registry, user.NewManager(db), "!")
	dispatcher.SetChannelPrefix("#chan", "-")

	if err := registry.Register(&testCommand{
		name:  "echo",
		level: database.LevelNormal,
		execute: func(ctx *Context) (*Response, error) {
			return NewResponse(ctx.ActivePrefix), nil
		},
	}); err != nil {
		t.Fatalf("Register echo failed: %v", err)
	}

	response, isCommand, err := dispatcher.Dispatch("alice", "", "#chan", "-echo", false)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if !isCommand {
		t.Fatal("expected dispatch to identify the command")
	}
	if response == nil || response.Message != "-" {
		t.Fatalf("expected active prefix '-' in response, got %#v", response)
	}
}
