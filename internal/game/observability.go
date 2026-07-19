package game

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sync"
)

// ActionEvent is the exhaustive allowlist for routine game telemetry. It has
// no message, hostmask, password, prompt, token, arguments, or narration field.
type ActionEvent struct {
	RequestID             string `json:"request_id"`
	Network               string `json:"network"`
	SessionRef            string `json:"session_ref"`
	ActionType            string `json:"action_type"`
	PreRevision           int64  `json:"pre_revision"`
	PostRevision          int64  `json:"post_revision"`
	LatencyMilliseconds   int64  `json:"latency_ms"`
	ResultCategory        string `json:"result_category"`
	ErrorCategory         string `json:"error_category,omitempty"`
	ConfigurationRevision int64  `json:"configuration_revision"`
	ContentPolicyRevision int64  `json:"content_policy_revision"`
}

// ActionObserver receives only privacy-projected action events.
type ActionObserver interface {
	ObserveGameAction(ActionEvent)
}

// GameTelemetry writes structured JSON and keeps aggregate category counters.
type GameTelemetry struct {
	mu       sync.Mutex
	writer   io.Writer
	counters map[string]uint64
}

func NewGameTelemetry(writer io.Writer) *GameTelemetry {
	return &GameTelemetry{writer: writer, counters: make(map[string]uint64)}
}

func (t *GameTelemetry) ObserveGameAction(event ActionEvent) {
	if t == nil {
		return
	}
	key := event.ActionType + "|" + event.ResultCategory + "|" + event.ErrorCategory
	t.mu.Lock()
	t.counters[key]++
	if t.writer != nil {
		payload, err := json.Marshal(event)
		if err == nil {
			_, _ = t.writer.Write(append(payload, '\n'))
		}
	}
	t.mu.Unlock()
}

func (t *GameTelemetry) MetricsSnapshot() map[string]uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make(map[string]uint64, len(t.counters))
	for key, value := range t.counters {
		result[key] = value
	}
	return result
}

func safeTelemetryCategory(value, fallback string) string {
	if idPattern.MatchString(value) {
		return value
	}
	return fallback
}

func safeSessionReference(network string, identity SessionIdentity) string {
	digest := sha256.Sum256([]byte(network + "\x1f" + string(identity.Kind) + "\x1f" + identity.Value))
	return "session-" + hex.EncodeToString(digest[:8])
}
