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
	now := time.Now()
	statePath := filepath.Join(t.TempDir(), "codex-reset-state.json")
	channels := []string{"#mathizen", "#mathizen.net", "##llm"}
	sender := &recordingSender{}
	source := &queuedSource{snapshots: []Snapshot{
		rateLimitSnapshot(76, 64, now.Add(4*time.Hour), now.Add(4*24*time.Hour)),
		rateLimitSnapshot(0, 0, now.Add(5*time.Hour), now.Add(7*24*time.Hour)),
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
	restartSource := &queuedSource{snapshots: []Snapshot{
		rateLimitSnapshot(0, 0, now.Add(5*time.Hour), now.Add(7*24*time.Hour)),
	}}
	restarted := newWithSource(testOptions(statePath, channels), sender, discardLogger{}, restartSource)
	restarted.check(context.Background(), &reloaded)
	for _, channel := range channels {
		if got := sender.count(channel); got != 1 {
			t.Fatalf("restart duplicated reset in %s: got %d messages", channel, got)
		}
	}
}

func TestScheduledRateLimitResetStaysSilent(t *testing.T) {
	now := time.Now()
	state := persistedState{
		Version:               stateVersion,
		RateLimitsInitialized: true,
		RateLimitWindows: map[string]persistedRateLimitWindow{
			"codex:primary": {UsedPercent: 80, ResetsAt: now.Add(-time.Minute).Unix()},
		},
	}

	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})
	watcher.applySnapshot(&state, rateLimitSnapshot(0, 0, now.Add(5*time.Hour), now.Add(7*24*time.Hour)), now)

	if len(state.Pending) != 0 {
		t.Fatalf("scheduled reset created announcement: %#v", state.Pending)
	}
}

func TestRateLimitUsageIncreaseStaysSilent(t *testing.T) {
	now := time.Now()
	state := persistedState{
		Version:               stateVersion,
		RateLimitsInitialized: true,
		RateLimitWindows: map[string]persistedRateLimitWindow{
			"codex:primary":   {UsedPercent: 20, ResetsAt: now.Add(4 * time.Hour).Unix()},
			"codex:secondary": {UsedPercent: 30, ResetsAt: now.Add(4 * 24 * time.Hour).Unix()},
		},
	}
	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})

	watcher.applySnapshot(&state, rateLimitSnapshot(21, 31, now.Add(4*time.Hour), now.Add(4*24*time.Hour)), now)

	if len(state.Pending) != 0 {
		t.Fatalf("ordinary usage increase created announcement: %#v", state.Pending)
	}
}

func TestEarlyRateLimitResetAnnounces(t *testing.T) {
	now := time.Now()
	state := persistedState{
		Version:               stateVersion,
		RateLimitsInitialized: true,
		RateLimitWindows: map[string]persistedRateLimitWindow{
			"codex:primary":   {UsedPercent: 72, ResetsAt: now.Add(4 * time.Hour).Unix()},
			"codex:secondary": {UsedPercent: 81, ResetsAt: now.Add(4 * 24 * time.Hour).Unix()},
		},
	}
	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})

	watcher.applySnapshot(&state, rateLimitSnapshot(0, 0, now.Add(5*time.Hour), now.Add(7*24*time.Hour)), now)

	if len(state.Pending) != 1 || state.Pending[0].Message != announcementText {
		t.Fatalf("pending = %#v, want one early-reset announcement", state.Pending)
	}
}

func TestRedeemedResetCreditStaysSilent(t *testing.T) {
	now := time.Now()
	state := persistedState{
		Version:                       stateVersion,
		RateLimitsInitialized:         true,
		ResetCreditsInitialized:       true,
		LastAvailableResetCreditCount: 1,
		RateLimitWindows: map[string]persistedRateLimitWindow{
			"codex:primary": {UsedPercent: 72, ResetsAt: now.Add(4 * time.Hour).Unix()},
		},
	}
	watcher := newWithSource(testOptions(filepath.Join(t.TempDir(), "state.json"), []string{"#mathizen"}), &recordingSender{}, discardLogger{}, &queuedSource{})
	snapshot := rateLimitSnapshot(0, 0, now.Add(5*time.Hour), now.Add(7*24*time.Hour))
	snapshot.ResetCredits = &ResetCreditsSummary{AvailableCount: 0}

	watcher.applySnapshot(&state, snapshot, now)

	if len(state.Pending) != 0 {
		t.Fatalf("redeemed reset credit created announcement: %#v", state.Pending)
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
		{"A Codex rate-limit reset credit is now available.", false},
		{"Codex limits have been reset for everyone.", true},
		{"Usage limits have been reset for all paid ChatGPT Work and Codex users.", true},
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

func rateLimitSnapshot(primaryUsed, secondaryUsed int, primaryReset, secondaryReset time.Time) Snapshot {
	primaryDuration := int64(300)
	secondaryDuration := int64(7 * 24 * 60)
	primaryResetAt := primaryReset.Unix()
	secondaryResetAt := secondaryReset.Unix()
	return Snapshot{RateLimits: map[string]RateLimitSnapshot{
		"codex": {
			LimitID: "codex",
			Primary: &RateLimitWindow{
				UsedPercent:        primaryUsed,
				WindowDurationMins: &primaryDuration,
				ResetsAt:           &primaryResetAt,
			},
			Secondary: &RateLimitWindow{
				UsedPercent:        secondaryUsed,
				WindowDurationMins: &secondaryDuration,
				ResetsAt:           &secondaryResetAt,
			},
		},
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
