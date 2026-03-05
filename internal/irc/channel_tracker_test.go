package irc

import (
	"testing"

	"github.com/yourusername/lolo/internal/database"
)

type noopLogger struct{}

func (noopLogger) Info(_ string, _ ...interface{})    {}
func (noopLogger) Success(_ string, _ ...interface{}) {}
func (noopLogger) Warning(_ string, _ ...interface{}) {}
func (noopLogger) Error(_ string, _ ...interface{})   {}
func (noopLogger) ChannelMessage(_, _, _ string)      {}
func (noopLogger) PrivateMessage(_, _ string)         {}

func TestOnNamesEndReplacesSnapshotAndTracksBotVoice(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	channel := "##llm-bots"
	botNick := "Lolo"
	tracker := NewChannelTracker(db, noopLogger{}, botNick)

	// Seed stale state to emulate drift from missed PART/QUIT events.
	if err := db.SetBotChannelStatus(channel, true, false, false, false); err != nil {
		t.Fatalf("SetBotChannelStatus failed: %v", err)
	}
	if err := db.UpsertChannelUser(channel, botNick, false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser(bot) failed: %v", err)
	}
	if err := db.UpsertChannelUser(channel, "stale-user", false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser(stale-user) failed: %v", err)
	}

	tracker.OnNamesReply(channel, []string{"+Lolo", "alice"})
	tracker.OnNamesReply(channel, []string{"bob", "carol"})
	tracker.OnNamesEnd(channel)

	status, err := db.GetBotChannelStatus(channel)
	if err != nil {
		t.Fatalf("GetBotChannelStatus failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected channel status, got nil")
	}
	if status.UserCount != 4 {
		t.Fatalf("expected user_count=4, got %d", status.UserCount)
	}
	if status.VoiceCount != 1 {
		t.Fatalf("expected voice_count=1, got %d", status.VoiceCount)
	}
	if !status.IsVoice {
		t.Fatal("expected bot channel status to report voice=true")
	}

	stale, err := db.GetChannelUser(channel, "stale-user")
	if err != nil {
		t.Fatalf("GetChannelUser(stale-user) failed: %v", err)
	}
	if stale != nil {
		t.Fatal("expected stale-user to be removed by names snapshot")
	}

	bot, err := db.GetChannelUser(channel, botNick)
	if err != nil {
		t.Fatalf("GetChannelUser(bot) failed: %v", err)
	}
	if bot == nil || !bot.IsVoice {
		t.Fatal("expected bot user to have voice=true after names snapshot")
	}
}
