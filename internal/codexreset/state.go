package codexreset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const stateVersion = 1

type persistedState struct {
	Version                       int                                 `json:"version"`
	RateLimitsInitialized         bool                                `json:"rate_limits_initialized"`
	RateLimitWindows              map[string]persistedRateLimitWindow `json:"rate_limit_windows,omitempty"`
	ResetCreditsInitialized       bool                                `json:"reset_credits_initialized"`
	LastAvailableResetCreditCount int64                               `json:"last_available_reset_credit_count"`
	WorkspaceMessagesInitialized  bool                                `json:"workspace_messages_initialized"`
	KnownWorkspaceMessageIDs      []string                            `json:"known_workspace_message_ids,omitempty"`
	LastCheckedAt                 int64                               `json:"last_checked_at"`
	LastAnnouncementAt            int64                               `json:"last_announcement_at"`
	Pending                       []pendingAnnouncement               `json:"pending,omitempty"`
}

type persistedRateLimitWindow struct {
	UsedPercent        int   `json:"used_percent"`
	WindowDurationMins int64 `json:"window_duration_mins,omitempty"`
	ResetsAt           int64 `json:"resets_at,omitempty"`
}

type pendingAnnouncement struct {
	Key               string   `json:"key"`
	Message           string   `json:"message"`
	CreatedAt         int64    `json:"created_at"`
	DeliveredChannels []string `json:"delivered_channels,omitempty"`
}

func loadState(path string) (persistedState, error) {
	state := persistedState{Version: stateVersion}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedState{Version: stateVersion}, fmt.Errorf("decode state: %w", err)
	}
	if state.Version != stateVersion {
		return persistedState{Version: stateVersion}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	return state, nil
}

func saveState(path string, state persistedState) error {
	state.Version = stateVersion
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	file, err := os.CreateTemp(directory, ".codex-reset-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	temporaryPath := file.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure temporary state: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode state: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
