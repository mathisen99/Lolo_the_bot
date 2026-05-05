package callback

import (
	"strings"
	"testing"

	"github.com/yourusername/lolo/internal/database"
)

func TestExecuteChannelInfoUsesRequestedNetwork(t *testing.T) {
	db, cleanup := database.NewTestDB(t)
	defer cleanup()

	if err := db.SetBotChannelStatusForNetwork("rizon", "#robots", true, false, false, false); err != nil {
		t.Fatalf("SetBotChannelStatusForNetwork failed: %v", err)
	}
	if err := db.UpsertChannelUserForNetwork("rizon", "#robots", "alice", false, false, false); err != nil {
		t.Fatalf("UpsertChannelUserForNetwork failed: %v", err)
	}

	server := NewServer(nil, nil, 0)
	server.SetDatabase(db)

	output, err := server.executeChannelInfo("rizon", []string{"#robots"})
	if err != nil {
		t.Fatalf("executeChannelInfo failed: %v", err)
	}
	if !strings.Contains(output, "Channel rizon/#robots: users=1") {
		t.Fatalf("expected rizon channel info, got %q", output)
	}

	output, err = server.executeChannelInfo("libera", []string{"#robots"})
	if err != nil {
		t.Fatalf("executeChannelInfo(libera) failed: %v", err)
	}
	if !strings.Contains(output, "Bot is not in channel libera/#robots") {
		t.Fatalf("expected libera miss diagnostic, got %q", output)
	}
}
