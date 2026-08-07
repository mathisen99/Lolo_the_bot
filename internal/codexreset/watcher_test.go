package codexreset

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type queuedSource struct {
	mu        sync.Mutex
	snapshots []Snapshot
	err       error
}

func (s *queuedSource) Read(context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Snapshot{}, s.err
	}
	if len(s.snapshots) == 0 {
		return Snapshot{}, errors.New("no queued snapshot")
	}
	snapshot := s.snapshots[0]
	s.snapshots = s.snapshots[1:]
	return snapshot, nil
}

type recordingSender struct {
	mu           sync.Mutex
	messages     map[string][]string
	failuresLeft map[string]int
}

func (s *recordingSender) SendMessage(target, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failuresLeft[target] > 0 {
		s.failuresLeft[target]--
		return errors.New("temporary IRC failure")
	}
	if s.messages == nil {
		s.messages = make(map[string][]string)
	}
	s.messages[target] = append(s.messages[target], message)
	return nil
}

func (s *recordingSender) count(target string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages[target])
}

type discardLogger struct{}

func (discardLogger) Info(string, ...interface{})    {}
func (discardLogger) Success(string, ...interface{}) {}
func (discardLogger) Warning(string, ...interface{}) {}

func TestWatcherBaselinesThenAnnouncesNewResetOnceAcrossRestart(t *testing.T) {
	now := time.Now().Unix()
	existing := ResetCredit{ID: "existing", ResetType: "codexRateLimits", Status: "available", GrantedAt: now - 3600}
	newCredit := ResetCredit{ID: "new", ResetType: "codexRateLimits", Status: "available", GrantedAt: now}
	statePath := filepath.Join(t.TempDir(), "codex-reset-state.json")
	channels := []string{"#mathizen", "#mathizen.net", "##llm"}
	sender := &recordingSender{}
	source := &queuedSource{snapshots: []Snapshot{
		resetSnapshot(1, existing),
		resetSnapshot(2, existing, newCredit),
	}}
	watcher := newWithSource(testOptions(statePath, channels), sender, discardLogger{}, source)

	state := persistedState{Version: stateVersion}
	watcher.check(context.Background(), &state)
	for _, channel := range channels {
		if got := sender.count(channel); got != 0 {
			t.Fatalf("baseline sent %d messages to %s, want 0", got, channel)
		}
	}

	watcher.check(context.Background(), &state)
	for _, channel := range channels {
		if got := sender.count(channel); got != 1 {
			t.Fatalf("new reset sent %d messages to %s, want 1", got, channel)
		}
	}
	if len(state.Pending) != 0 {
		t.Fatalf("pending announcements = %#v, want none after successful delivery", state.Pending)
	}

	reloaded, err := loadState(statePath)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	restartSource := &queuedSource{snapshots: []Snapshot{resetSnapshot(2, existing, newCredit)}}
	restarted := newWithSource(testOptions(statePath, channels), sender, discardLogger{}, restartSource)
	restarted.check(context.Background(), &reloaded)
	for _, channel := range channels {
		if got := sender.count(channel); got != 1 {
			t.Fatalf("restart duplicated reset in %s: got %d messages", channel, got)
		}
	}
}

func TestUnchangedAndDecreasedResetCreditCountsStaySilent(t *testing.T) {
	state := persistedState{
		Version:                       stateVersion,
		ResetCreditsInitialized:       true,
		LastAvailableResetCreditCount: 2,
		LastCheckedAt:                 time.Now().Add(-time.Minute).Unix(),
	}
	now := time.Now()

	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})
	watcher.applySnapshot(&state, Snapshot{ResetCredits: &ResetCreditsSummary{AvailableCount: 2}}, now)
	watcher.applySnapshot(&state, Snapshot{ResetCredits: &ResetCreditsSummary{AvailableCount: 0}}, now.Add(time.Minute))

	if len(state.Pending) != 0 {
		t.Fatalf("ordinary or decreasing state created announcement: %#v", state.Pending)
	}
}

func TestOldCreditDetailRotationDoesNotAnnounce(t *testing.T) {
	now := time.Now()
	oldCredit := ResetCredit{
		ID:        "old-but-previously-hidden",
		ResetType: "codexRateLimits",
		Status:    "available",
		GrantedAt: now.Add(-24 * time.Hour).Unix(),
	}
	state := persistedState{
		Version:                       stateVersion,
		ResetCreditsInitialized:       true,
		LastAvailableResetCreditCount: 1,
		LastCheckedAt:                 now.Add(-time.Minute).Unix(),
	}
	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})

	watcher.applySnapshot(&state, resetSnapshot(1, oldCredit), now)

	if len(state.Pending) != 0 {
		t.Fatalf("old rotated credit detail created announcement: %#v", state.Pending)
	}
}

func TestCountOnlyResetCreditIncreaseAnnounces(t *testing.T) {
	now := time.Now()
	state := persistedState{
		Version:                       stateVersion,
		ResetCreditsInitialized:       true,
		LastAvailableResetCreditCount: 1,
		LastCheckedAt:                 now.Add(-time.Minute).Unix(),
	}
	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})

	watcher.applySnapshot(&state, Snapshot{ResetCredits: &ResetCreditsSummary{AvailableCount: 2}}, now)

	if len(state.Pending) != 1 || state.Pending[0].Message != announcementText {
		t.Fatalf("pending = %#v, want one special-reset announcement", state.Pending)
	}
}

func TestWorkspaceMessageFilterRejectsRoutineResets(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"Your Codex weekly limit will reset on Monday.", false},
		{"Your Codex five-hour limit resets at 15:00.", false},
		{"Your monthly Codex usage limit was reset.", false},
		{"OpenAI issued a special reset for Codex limits.", true},
		{"A Codex rate-limit reset credit is now available.", true},
		{"Codex limits have been reset for everyone.", true},
		{"General workspace maintenance is complete.", false},
	}

	for _, test := range tests {
		if got := isExplicitSpecialResetMessage(test.body); got != test.want {
			t.Errorf("isExplicitSpecialResetMessage(%q) = %v, want %v", test.body, got, test.want)
		}
	}
}

func TestPendingDeliveryRetriesOnlyFailedChannel(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	channels := []string{"#mathizen", "#mathizen.net", "##llm"}
	sender := &recordingSender{failuresLeft: map[string]int{"#mathizen.net": 1}}
	watcher := newWithSource(testOptions(statePath, channels), sender, discardLogger{}, &queuedSource{})
	state := persistedState{
		Version: stateVersion,
		Pending: []pendingAnnouncement{{Key: "credit:new", Message: announcementText}},
	}

	watcher.deliverPending(context.Background(), &state)
	if len(state.Pending) != 1 {
		t.Fatalf("pending count after partial failure = %d, want 1", len(state.Pending))
	}
	if sender.count("#mathizen") != 1 || sender.count("##llm") != 1 || sender.count("#mathizen.net") != 0 {
		t.Fatalf("unexpected first delivery counts: %#v", sender.messages)
	}

	watcher.deliverPending(context.Background(), &state)
	if len(state.Pending) != 0 {
		t.Fatalf("pending count after retry = %d, want 0", len(state.Pending))
	}
	for _, channel := range channels {
		if got := sender.count(channel); got != 1 {
			t.Fatalf("delivery count for %s = %d, want 1", channel, got)
		}
	}
}

func TestWatcherStartAndStop(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	source := &queuedSource{snapshots: []Snapshot{{}}}
	watcher := newWithSource(Options{
		StatePath:    statePath,
		Channels:     []string{"#mathizen"},
		PollInterval: time.Hour,
		QueryTimeout: time.Second,
	}, &recordingSender{}, discardLogger{}, source)

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func resetSnapshot(count int64, credits ...ResetCredit) Snapshot {
	copyOfCredits := append([]ResetCredit(nil), credits...)
	return Snapshot{ResetCredits: &ResetCreditsSummary{
		AvailableCount: count,
		Credits:        &copyOfCredits,
	}}
}

func testOptions(statePath string, channels []string) Options {
	return Options{
		StatePath:    statePath,
		Channels:     channels,
		PollInterval: time.Hour,
		QueryTimeout: time.Second,
	}
}
