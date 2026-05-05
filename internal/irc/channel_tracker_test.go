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

func TestOnChannelMessageTouchesMembershipWithoutClearingModes(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	channel := "#robots"
	tracker := NewChannelTrackerForNetwork(db, noopLogger{}, "rizon", "Lolo")

	if err := db.SetBotChannelStatusForNetwork("rizon", channel, true, false, false, true); err != nil {
		t.Fatalf("SetBotChannelStatusForNetwork failed: %v", err)
	}
	if err := db.UpsertChannelUserForNetwork("rizon", channel, "alice", true, false, false); err != nil {
		t.Fatalf("UpsertChannelUserForNetwork failed: %v", err)
	}

	tracker.OnChannelMessage(channel, "alice")

	status, err := db.GetBotChannelStatusForNetwork("rizon", channel)
	if err != nil {
		t.Fatalf("GetBotChannelStatusForNetwork failed: %v", err)
	}
	if status == nil || !status.IsJoined {
		t.Fatal("expected bot status to remain joined after channel traffic")
	}
	if !status.IsVoice {
		t.Fatal("expected bot voice mode to be preserved")
	}

	alice, err := db.GetChannelUserForNetwork("rizon", channel, "alice")
	if err != nil {
		t.Fatalf("GetChannelUserForNetwork failed: %v", err)
	}
	if alice == nil || !alice.IsOp {
		t.Fatal("expected speaker op mode to be preserved")
	}
}
