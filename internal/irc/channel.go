package irc

import (
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/output"
)

// ChannelManager manages channel state and operations
type ChannelManager struct {
	db             *database.DB
	logger         output.Logger
	client         *Client
	joinedChannels map[string]bool
	channelStates  map[string]bool // channel -> enabled/disabled
	mu             sync.RWMutex
	autoJoinList   []string
}

// NewChannelManager creates a new channel manager
func NewChannelManager(db *database.DB, logger output.Logger, client *Client, autoJoinList []string) *ChannelManager {
	return &ChannelManager{
		db:             db,
		logger:         logger,
		client:         client,
		joinedChannels: make(map[string]bool),
		channelStates:  make(map[string]bool),
		autoJoinList:   autoJoinList,
	}
}

// LoadChannelStates loads channel enabled/disabled states from the database
func (cm *ChannelManager) LoadChannelStates() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	states, err := cm.db.ListChannelStates()
	if err != nil {
		return fmt.Errorf("failed to load channel states: %w", err)
	}

	// Load states into memory
	for _, state := range states {
		cm.channelStates[state.Channel] = state.Enabled
		cm.logger.Info("Loaded channel state: %s (enabled: %v)", state.Channel, state.Enabled)
	}

	return nil
}

// IsChannelEnabled checks if a channel is enabled
// Returns true by default if no state is stored
func (cm *ChannelManager) IsChannelEnabled(channel string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check in-memory cache first
	if enabled, exists := cm.channelStates[channel]; exists {
		return enabled
	}

	// Default to enabled if not found
	return true
}

// SetChannelEnabled sets the enabled/disabled state for a channel
func (cm *ChannelManager) SetChannelEnabled(channel string, enabled bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Update database
	err := cm.db.SetChannelState(channel, enabled)
	if err != nil {
		return fmt.Errorf("failed to set channel state: %w", err)
	}

	// Update in-memory cache
	cm.channelStates[channel] = enabled
	cm.logger.Info("Channel %s state updated: enabled=%v", channel, enabled)

	return nil
}

// JoinChannel joins a channel if it's enabled
func (cm *ChannelManager) JoinChannel(channel string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if channel is enabled
	enabled := cm.getChannelEnabledLocked(channel)
	if !enabled {
		cm.logger.Warning("Channel %s is disabled, skipping join", channel)
		return fmt.Errorf("channel %s is disabled", channel)
	}

	// Join the channel
	err := cm.client.JoinChannel(channel)
	if err != nil {
		return fmt.Errorf("failed to join channel: %w", err)
	}

	return nil
}

// PartChannel leaves a channel
func (cm *ChannelManager) PartChannel(channel, message string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Leave the channel
	err := cm.client.PartChannel(channel, message)
	if err != nil {
		return fmt.Errorf("failed to part channel: %w", err)
	}

	// Remove from joined channels
	delete(cm.joinedChannels, channel)

	return nil
}

// JoinAutoJoinChannels joins all configured auto-join channels that are enabled
// Joins are staggered to avoid excess flood disconnects
func (cm *ChannelManager) JoinAutoJoinChannels() error {
	// Wait before starting to join channels (let connection settle)
	time.Sleep(3 * time.Second)

	for i, channel := range cm.autoJoinList {
		cm.mu.Lock()
		// Check if channel is enabled
		enabled := cm.getChannelEnabledLocked(channel)
		if !enabled {
			cm.logger.Warning("Skipping auto-join for disabled channel: %s", channel)
			cm.mu.Unlock()
			continue
		}

		// Join the channel
		err := cm.client.JoinChannel(channel)
		if err != nil {
			cm.logger.Error("Failed to join channel %s: %v", channel, err)
			cm.mu.Unlock()
			continue
		}

		cm.logger.Info("Auto-joining channel: %s", channel)
		cm.mu.Unlock()

		// Delay between joins to avoid flood (except after last channel)
		if i < len(cm.autoJoinList)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

// RejoinChannels rejoins all previously joined channels that are enabled
// Joins are staggered to avoid excess flood disconnects
func (cm *ChannelManager) RejoinChannels() error {
	// Wait before starting to rejoin channels (let connection settle)
	time.Sleep(3 * time.Second)

	// Get list of channels to rejoin
	cm.mu.RLock()
	channels := make([]string, 0, len(cm.joinedChannels))
	for channel := range cm.joinedChannels {
		channels = append(channels, channel)
	}
	cm.mu.RUnlock()

	for i, channel := range channels {
		cm.mu.Lock()
		// Check if channel is still enabled
		enabled := cm.getChannelEnabledLocked(channel)
		if !enabled {
			cm.logger.Warning("Skipping rejoin for disabled channel: %s", channel)
			cm.mu.Unlock()
			continue
		}

		// Rejoin the channel
		err := cm.client.JoinChannel(channel)
		if err != nil {
			cm.logger.Error("Failed to rejoin channel %s: %v", channel, err)
			cm.mu.Unlock()
			continue
		}

		cm.logger.Info("Rejoining channel: %s", channel)
		cm.mu.Unlock()

		// Delay between joins to avoid flood (except after last channel)
		if i < len(channels)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

// OnJoin is called when the bot successfully joins a channel
func (cm *ChannelManager) OnJoin(channel string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.joinedChannels[channel] = true
	cm.logger.Success("Joined channel: %s", channel)

	// Ensure channel is marked as enabled in database (default state)
	if _, exists := cm.channelStates[channel]; !exists {
		// Set default enabled state
		err := cm.db.SetChannelState(channel, true)
		if err != nil {
			cm.logger.Error("Failed to set default channel state for %s: %v", channel, err)
		} else {
			cm.channelStates[channel] = true
		}
	}
}

// OnPart is called when the bot leaves a channel
func (cm *ChannelManager) OnPart(channel string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.joinedChannels, channel)
	cm.logger.Info("Left channel: %s", channel)
}

// OnKick is called when the bot is kicked from a channel
func (cm *ChannelManager) OnKick(channel string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.joinedChannels, channel)
	cm.logger.Warning("Kicked from channel: %s", channel)
}

// IsJoined checks if the bot is currently in a channel
func (cm *ChannelManager) IsJoined(channel string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.joinedChannels[channel]
}

// GetJoinedChannels returns a list of currently joined channels
func (cm *ChannelManager) GetJoinedChannels() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	channels := make([]string, 0, len(cm.joinedChannels))
	for channel := range cm.joinedChannels {
		channels = append(channels, channel)
	}

	return channels
}

// GetEnabledChannels returns a list of enabled channels
func (cm *ChannelManager) GetEnabledChannels() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	channels := make([]string, 0)
	for channel, enabled := range cm.channelStates {
		if enabled {
			channels = append(channels, channel)
		}
	}

	return channels
}

// getChannelEnabledLocked checks if a channel is enabled (must be called with lock held)
func (cm *ChannelManager) getChannelEnabledLocked(channel string) bool {
	// Check in-memory cache
	if enabled, exists := cm.channelStates[channel]; exists {
		return enabled
	}

	// Default to enabled if not found
	return true
}
