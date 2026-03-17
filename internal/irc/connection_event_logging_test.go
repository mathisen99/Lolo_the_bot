package irc

import (
	"strings"
	"testing"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
	"gopkg.in/irc.v4"
)

func newTestConnectionManager(t *testing.T) (*ConnectionManager, *database.DB, func()) {
	t.Helper()

	db, cleanup := database.NewTestDB(t)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Nickname: "Lolo",
		},
		Bot: config.BotConfig{
			Channels: []string{"#alpha", "#beta"},
		},
		Limits: config.LimitsConfig{
			ReconnectDelayMin: 1,
			ReconnectDelayMax: 2,
		},
	}

	cm := NewConnectionManager(cfg, noopLogger{}, db, user.NewManager(db))
	cm.SetChannelUserTracker(NewChannelTracker(db, noopLogger{}, cfg.Server.Nickname))

	return cm, db, cleanup
}

func mustSeedChannelUser(t *testing.T, db *database.DB, channel, nick string) {
	t.Helper()

	if err := db.UpsertChannelUser(channel, nick, false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser(%s, %s) failed: %v", channel, nick, err)
	}
}

func countEvents(t *testing.T, db *database.DB, eventType, channel string) int {
	t.Helper()

	var count int
	err := db.Conn().QueryRow(`
		SELECT COUNT(*)
		FROM messages
		WHERE event_type = ? AND LOWER(channel) = LOWER(?)
	`, eventType, channel).Scan(&count)
	if err != nil {
		t.Fatalf("countEvents query failed: %v", err)
	}

	return count
}

func countGlobalEvents(t *testing.T, db *database.DB, eventType string) int {
	t.Helper()

	var count int
	err := db.Conn().QueryRow(`
		SELECT COUNT(*)
		FROM messages
		WHERE event_type = ? AND (channel = '' OR channel IS NULL)
	`, eventType).Scan(&count)
	if err != nil {
		t.Fatalf("countGlobalEvents query failed: %v", err)
	}

	return count
}

func latestEventContent(t *testing.T, db *database.DB, eventType, channel string) string {
	t.Helper()

	var content string
	err := db.Conn().QueryRow(`
		SELECT content
		FROM messages
		WHERE event_type = ? AND LOWER(channel) = LOWER(?)
		ORDER BY timestamp DESC, id DESC
		LIMIT 1
	`, eventType, channel).Scan(&content)
	if err != nil {
		t.Fatalf("latestEventContent query failed: %v", err)
	}

	return content
}

func TestHandleNickChangeLogsPerTrackedChannel(t *testing.T) {
	cm, db, cleanup := newTestConnectionManager(t)
	defer cleanup()

	mustSeedChannelUser(t, db, "#alpha", "alice")
	mustSeedChannelUser(t, db, "#beta", "alice")

	cm.handleNickChange(&irc.Message{
		Prefix: &irc.Prefix{
			Name: "alice",
			User: "alice",
			Host: "example.test",
		},
		Params: []string{"alice_"},
	})

	if got := countEvents(t, db, database.EventTypeNickChange, "#alpha"); got != 1 {
		t.Fatalf("expected 1 NICK event in #alpha, got %d", got)
	}
	if got := countEvents(t, db, database.EventTypeNickChange, "#beta"); got != 1 {
		t.Fatalf("expected 1 NICK event in #beta, got %d", got)
	}
	if got := countGlobalEvents(t, db, database.EventTypeNickChange); got != 0 {
		t.Fatalf("expected 0 global NICK events, got %d", got)
	}

	if content := latestEventContent(t, db, database.EventTypeNickChange, "#alpha"); !strings.Contains(content, "alice is now known as alice_") {
		t.Fatalf("unexpected NICK event content: %q", content)
	}

	renamed, err := db.GetChannelUser("#alpha", "alice_")
	if err != nil {
		t.Fatalf("GetChannelUser for renamed user failed: %v", err)
	}
	if renamed == nil {
		t.Fatal("expected channel tracker to rename alice to alice_")
	}
}

func TestHandleQuitLogsPerTrackedChannel(t *testing.T) {
	cm, db, cleanup := newTestConnectionManager(t)
	defer cleanup()

	mustSeedChannelUser(t, db, "#alpha", "alice")
	mustSeedChannelUser(t, db, "#beta", "alice")

	cm.handleQuit(&irc.Message{
		Prefix: &irc.Prefix{
			Name: "alice",
			User: "alice",
			Host: "example.test",
		},
		Params: []string{"Client exited"},
	})

	if got := countEvents(t, db, database.EventTypeQuit, "#alpha"); got != 1 {
		t.Fatalf("expected 1 QUIT event in #alpha, got %d", got)
	}
	if got := countEvents(t, db, database.EventTypeQuit, "#beta"); got != 1 {
		t.Fatalf("expected 1 QUIT event in #beta, got %d", got)
	}
	if got := countGlobalEvents(t, db, database.EventTypeQuit); got != 0 {
		t.Fatalf("expected 0 global QUIT events, got %d", got)
	}

	if content := latestEventContent(t, db, database.EventTypeQuit, "#alpha"); !strings.Contains(content, "alice has quit (Client exited)") {
		t.Fatalf("unexpected QUIT event content: %q", content)
	}

	channels, err := db.FindUserChannels("alice")
	if err != nil {
		t.Fatalf("FindUserChannels failed: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("expected alice to be removed from all channels after quit, got %v", channels)
	}
}

func TestHandleQuitFallsBackToGlobalEventWithoutTrackedChannels(t *testing.T) {
	cm, db, cleanup := newTestConnectionManager(t)
	defer cleanup()

	cm.handleQuit(&irc.Message{
		Prefix: &irc.Prefix{
			Name: "ghost",
			User: "ghost",
			Host: "example.test",
		},
	})

	if got := countEvents(t, db, database.EventTypeQuit, "#alpha"); got != 0 {
		t.Fatalf("expected 0 QUIT events in #alpha, got %d", got)
	}
	if got := countEvents(t, db, database.EventTypeQuit, "#beta"); got != 0 {
		t.Fatalf("expected 0 QUIT events in #beta, got %d", got)
	}
	if got := countGlobalEvents(t, db, database.EventTypeQuit); got != 1 {
		t.Fatalf("expected 1 global QUIT fallback event, got %d", got)
	}
}

func TestHandlePartLogsChannelEventForOtherUsers(t *testing.T) {
	cm, db, cleanup := newTestConnectionManager(t)
	defer cleanup()

	mustSeedChannelUser(t, db, "#alpha", "alice")

	cm.handlePart(&irc.Message{
		Prefix: &irc.Prefix{
			Name: "alice",
			User: "alice",
			Host: "example.test",
		},
		Params: []string{"#alpha", "brb"},
	})

	if got := countEvents(t, db, database.EventTypePart, "#alpha"); got != 1 {
		t.Fatalf("expected 1 PART event in #alpha, got %d", got)
	}
	if got := countGlobalEvents(t, db, database.EventTypePart); got != 0 {
		t.Fatalf("expected 0 global PART events, got %d", got)
	}

	if content := latestEventContent(t, db, database.EventTypePart, "#alpha"); !strings.Contains(content, "alice has left #alpha (brb)") {
		t.Fatalf("unexpected PART event content: %q", content)
	}
}
