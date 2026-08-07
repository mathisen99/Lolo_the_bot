// Package codexreset announces special Codex rate-limit reset grants without
// confusing them with normal five-hour, weekly, or monthly quota resets.
package codexreset

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	announcementText        = "OpenAI has issued a special Codex rate-limit reset. This is separate from normal scheduled limit windows."
	announcementDedupWindow = time.Hour
	maximumRememberedIDs    = 500
)

type Sender interface {
	SendMessage(target, message string) error
}

type Logger interface {
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warning(format string, args ...interface{})
}

type Options struct {
	CodexPath    string
	StatePath    string
	Channels     []string
	PollInterval time.Duration
	QueryTimeout time.Duration
}

type Watcher struct {
	options Options
	sender  Sender
	logger  Logger
	source  snapshotSource

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func New(options Options, sender Sender, logger Logger) *Watcher {
	return newWithSource(options, sender, logger, &AppServerSource{CodexPath: options.CodexPath})
}

func newWithSource(options Options, sender Sender, logger Logger, source snapshotSource) *Watcher {
	return &Watcher{options: options, sender: sender, logger: logger, source: source}
}

func (w *Watcher) Start() error {
	if w.sender == nil || w.logger == nil || w.source == nil {
		return fmt.Errorf("Codex reset watcher dependencies are incomplete")
	}
	if strings.TrimSpace(w.options.StatePath) == "" {
		return fmt.Errorf("Codex reset watcher state path is empty")
	}
	if len(w.options.Channels) == 0 {
		return fmt.Errorf("Codex reset watcher has no announcement channels")
	}
	if w.options.PollInterval <= 0 || w.options.QueryTimeout <= 0 {
		return fmt.Errorf("Codex reset watcher intervals must be positive")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return fmt.Errorf("Codex reset watcher is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	w.running = true
	go w.run(ctx, w.done)
	return nil
}

func (w *Watcher) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.running = false
	w.mu.Unlock()

	cancel()
	<-done
	return nil
}

func (w *Watcher) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	state, err := loadState(w.options.StatePath)
	if err != nil {
		w.logger.Warning("Codex reset watcher: state could not be loaded; establishing a fresh baseline: %v", err)
	}

	w.check(ctx, &state)
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check(ctx, &state)
		}
	}
}

func (w *Watcher) check(ctx context.Context, state *persistedState) {
	if ctx.Err() != nil {
		return
	}
	w.deliverPending(ctx, state)

	queryCtx, cancel := context.WithTimeout(ctx, w.options.QueryTimeout)
	snapshot, err := w.source.Read(queryCtx)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Warning("Codex reset watcher: account-state query failed: %v", err)
		}
		return
	}

	now := time.Now()
	w.applySnapshot(state, snapshot, now)
	if err := saveState(w.options.StatePath, *state); err != nil {
		w.logger.Warning("Codex reset watcher: state could not be saved: %v", err)
		return
	}
	w.deliverPending(ctx, state)
}

func (w *Watcher) applySnapshot(state *persistedState, snapshot Snapshot, now time.Time) {
	signals := make([]string, 0, 2)
	if snapshot.ResetCredits != nil {
		signals = append(signals, applyResetCredits(state, *snapshot.ResetCredits, now)...)
	}
	if snapshot.WorkspaceMessages != nil && snapshot.WorkspaceMessages.FeatureEnabled {
		signals = append(signals, applyWorkspaceMessages(state, *snapshot.WorkspaceMessages)...)
	}

	state.LastCheckedAt = now.Unix()
	if len(signals) == 0 || hasPendingAnnouncement(state) {
		return
	}
	if state.LastAnnouncementAt > 0 && now.Sub(time.Unix(state.LastAnnouncementAt, 0)) < announcementDedupWindow {
		return
	}

	state.Pending = append(state.Pending, pendingAnnouncement{
		Key:       signals[0],
		Message:   announcementText,
		CreatedAt: now.Unix(),
	})
	state.LastAnnouncementAt = now.Unix()
}

func applyResetCredits(state *persistedState, summary ResetCreditsSummary, now time.Time) []string {
	credits := []ResetCredit(nil)
	if summary.Credits != nil {
		credits = *summary.Credits
	}

	if !state.ResetCreditsInitialized {
		for _, credit := range credits {
			state.KnownCreditIDs = rememberID(state.KnownCreditIDs, credit.ID)
		}
		state.LastAvailableResetCreditCount = summary.AvailableCount
		state.ResetCreditsInitialized = true
		return nil
	}

	newGrant := false
	key := ""
	for _, credit := range credits {
		known := containsID(state.KnownCreditIDs, credit.ID)
		state.KnownCreditIDs = rememberID(state.KnownCreditIDs, credit.ID)
		if known || credit.ID == "" || credit.ResetType != "codexRateLimits" || credit.Status != "available" {
			continue
		}
		// A previously unseen but old row can appear when the backend rotates a
		// capped detail list. Only a grant newer than our prior observation is
		// strong enough to announce.
		if state.LastCheckedAt > 0 && credit.GrantedAt < state.LastCheckedAt-5 {
			continue
		}
		newGrant = true
		if key == "" {
			key = "credit:" + credit.ID
		}
	}

	if summary.AvailableCount > state.LastAvailableResetCreditCount {
		newGrant = true
		if key == "" {
			key = fmt.Sprintf("credit-count:%d:%d", now.Unix(), summary.AvailableCount)
		}
	}
	state.LastAvailableResetCreditCount = summary.AvailableCount
	if newGrant {
		return []string{key}
	}
	return nil
}

func applyWorkspaceMessages(state *persistedState, snapshot WorkspaceMessagesSnapshot) []string {
	if !state.WorkspaceMessagesInitialized {
		for _, message := range snapshot.Messages {
			state.KnownWorkspaceMessageIDs = rememberID(state.KnownWorkspaceMessageIDs, message.ID)
		}
		state.WorkspaceMessagesInitialized = true
		return nil
	}

	for _, message := range snapshot.Messages {
		known := containsID(state.KnownWorkspaceMessageIDs, message.ID)
		state.KnownWorkspaceMessageIDs = rememberID(state.KnownWorkspaceMessageIDs, message.ID)
		if !known && isExplicitSpecialResetMessage(message.Body) {
			return []string{"workspace-message:" + message.ID}
		}
	}
	return nil
}

func isExplicitSpecialResetMessage(body string) bool {
	text := strings.ToLower(strings.Join(strings.Fields(body), " "))
	if !strings.Contains(text, "codex") || !strings.Contains(text, "limit") || !strings.Contains(text, "reset") {
		return false
	}
	for _, routine := range []string{"five-hour", "5-hour", "weekly", "monthly", "scheduled", "resets at", "will reset"} {
		if strings.Contains(text, routine) {
			return false
		}
	}
	for _, explicit := range []string{
		"special reset",
		"reset credit",
		"limits have been reset for everyone",
		"limits have been reset for all users",
		"limit has been reset for everyone",
		"limit has been reset for all users",
	} {
		if strings.Contains(text, explicit) {
			return true
		}
	}
	return false
}

func (w *Watcher) deliverPending(ctx context.Context, state *persistedState) {
	if len(state.Pending) == 0 {
		return
	}

	for eventIndex := range state.Pending {
		event := &state.Pending[eventIndex]
		for _, channel := range w.options.Channels {
			if ctx.Err() != nil {
				return
			}
			if containsFold(event.DeliveredChannels, channel) {
				continue
			}
			if err := w.sender.SendMessage(channel, event.Message); err != nil {
				w.logger.Warning("Codex reset watcher: failed to notify %s: %v", channel, err)
				continue
			}
			event.DeliveredChannels = append(event.DeliveredChannels, channel)
			if err := saveState(w.options.StatePath, *state); err != nil {
				w.logger.Warning("Codex reset watcher: delivery state could not be saved: %v", err)
			}
			w.logger.Success("Codex reset watcher: notified %s", channel)
		}
	}

	remaining := state.Pending[:0]
	for _, event := range state.Pending {
		if !deliveredEverywhere(event, w.options.Channels) {
			remaining = append(remaining, event)
		}
	}
	state.Pending = remaining
	if err := saveState(w.options.StatePath, *state); err != nil {
		w.logger.Warning("Codex reset watcher: final delivery state could not be saved: %v", err)
	}
}

func hasPendingAnnouncement(state *persistedState) bool {
	return len(state.Pending) > 0
}

func deliveredEverywhere(event pendingAnnouncement, channels []string) bool {
	for _, channel := range channels {
		if !containsFold(event.DeliveredChannels, channel) {
			return false
		}
	}
	return true
}

func containsID(ids []string, candidate string) bool {
	for _, id := range ids {
		if id == candidate {
			return true
		}
	}
	return false
}

func rememberID(ids []string, id string) []string {
	if id == "" || containsID(ids, id) {
		return ids
	}
	ids = append(ids, id)
	if len(ids) > maximumRememberedIDs {
		ids = append([]string(nil), ids[len(ids)-maximumRememberedIDs:]...)
	}
	return ids
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}
