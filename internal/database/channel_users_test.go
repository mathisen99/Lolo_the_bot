package database

import "testing"

func TestReplaceChannelUsersSnapshotUpdatesCountsAndClearsStaleUsers(t *testing.T) {
	db, cleanup := NewTestDB(t)
	defer cleanup()

	channel := "##llm-bots"

	if err := db.SetBotChannelStatus(channel, true, false, false, false); err != nil {
		t.Fatalf("SetBotChannelStatus failed: %v", err)
	}

	if err := db.UpsertChannelUser(channel, "Lolo", false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser(Lolo) failed: %v", err)
	}
	if err := db.UpsertChannelUser(channel, "stale-user", false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser(stale-user) failed: %v", err)
	}

	snapshot := []ChannelUserEntry{
		{Nick: "Lolo", IsVoice: true},
		{Nick: "alice", IsOp: true},
		{Nick: "bob"},
	}
	if err := db.ReplaceChannelUsersSnapshot(channel, snapshot); err != nil {
		t.Fatalf("ReplaceChannelUsersSnapshot failed: %v", err)
	}

	status, err := db.GetBotChannelStatus(channel)
	if err != nil {
		t.Fatalf("GetBotChannelStatus failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected channel status, got nil")
	}

	if status.UserCount != 3 {
		t.Fatalf("expected user_count=3, got %d", status.UserCount)
	}
	if status.OpCount != 1 {
		t.Fatalf("expected op_count=1, got %d", status.OpCount)
	}
	if status.VoiceCount != 1 {
		t.Fatalf("expected voice_count=1, got %d", status.VoiceCount)
	}

	stale, err := db.GetChannelUser(channel, "stale-user")
	if err != nil {
		t.Fatalf("GetChannelUser(stale-user) failed: %v", err)
	}
	if stale != nil {
		t.Fatal("expected stale-user to be removed by snapshot replacement")
	}

	bot, err := db.GetChannelUser(channel, "Lolo")
	if err != nil {
		t.Fatalf("GetChannelUser(Lolo) failed: %v", err)
	}
	if bot == nil || !bot.IsVoice {
		t.Fatal("expected bot user to be present with voice=true")
	}
}

func TestReplaceChannelUsersSnapshotCanClearChannel(t *testing.T) {
	db, cleanup := NewTestDB(t)
	defer cleanup()

	channel := "#empty"

	if err := db.SetBotChannelStatus(channel, true, false, false, false); err != nil {
		t.Fatalf("SetBotChannelStatus failed: %v", err)
	}
	if err := db.UpsertChannelUser(channel, "someone", false, false, false); err != nil {
		t.Fatalf("UpsertChannelUser failed: %v", err)
	}

	if err := db.ReplaceChannelUsersSnapshot(channel, nil); err != nil {
		t.Fatalf("ReplaceChannelUsersSnapshot(nil) failed: %v", err)
	}

	status, err := db.GetBotChannelStatus(channel)
	if err != nil {
		t.Fatalf("GetBotChannelStatus failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected channel status, got nil")
	}
	if status.UserCount != 0 || status.OpCount != 0 || status.VoiceCount != 0 {
		t.Fatalf("expected zero counts, got users=%d ops=%d voice=%d",
			status.UserCount, status.OpCount, status.VoiceCount)
	}
}
