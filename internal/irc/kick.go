package irc

import (
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// KickManager handles kick and ban events with exponential backoff
type KickManager struct {
	logger          output.Logger
	channelBackoffs map[string]*channelBackoff
	mu              sync.RWMutex
}

// channelBackoff tracks kick backoff state for a single channel
type channelBackoff struct {
	currentDelay time.Duration
	minDelay     time.Duration
	maxDelay     time.Duration
	kickCount    int
	banned       bool
	lastKick     time.Time
}

// NewKickManager creates a new kick manager
func NewKickManager(logger output.Logger) *KickManager {
	return &KickManager{
		logger:          logger,
		channelBackoffs: make(map[string]*channelBackoff),
	}
}

// OnKick handles a kick event for a channel
func (km *KickManager) OnKick(channel, reason string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Get or create backoff state for this channel
	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		backoff = &channelBackoff{
			currentDelay: 30 * time.Second, // Initial 30-second delay
			minDelay:     30 * time.Second,
			maxDelay:     10 * time.Minute,
			kickCount:    0,
			banned:       false,
		}
		km.channelBackoffs[channel] = backoff
	}

	// Increment kick count
	backoff.kickCount++
	backoff.lastKick = time.Now()

	// Log the kick with colored warning
	km.logger.Warning("Kicked from %s: %s (kick count: %d, will rejoin in %v)",
		channel, reason, backoff.kickCount, backoff.currentDelay)
}

// OnBan handles a ban event for a channel
func (km *KickManager) OnBan(channel string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Get or create backoff state for this channel
	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		backoff = &channelBackoff{
			currentDelay: 30 * time.Second,
			minDelay:     30 * time.Second,
			maxDelay:     10 * time.Minute,
			kickCount:    0,
			banned:       true,
		}
		km.channelBackoffs[channel] = backoff
	} else {
		backoff.banned = true
	}

	km.logger.Warning("Banned from %s - will not attempt to rejoin until ban is lifted", channel)
}

// GetRejoinDelay returns the delay before attempting to rejoin a channel
// Returns 0 if the channel is banned or doesn't have a backoff state
func (km *KickManager) GetRejoinDelay(channel string) time.Duration {
	km.mu.RLock()
	defer km.mu.RUnlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return 0
	}

	if backoff.banned {
		return 0 // Don't rejoin if banned
	}

	return backoff.currentDelay
}

// IsBanned returns whether the bot is banned from a channel
func (km *KickManager) IsBanned(channel string) bool {
	km.mu.RLock()
	defer km.mu.RUnlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return false
	}

	return backoff.banned
}

// IncreaseBackoff increases the backoff delay for a channel (exponential backoff)
func (km *KickManager) IncreaseBackoff(channel string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return
	}

	// Double the delay, up to the maximum
	backoff.currentDelay = backoff.currentDelay * 2
	if backoff.currentDelay > backoff.maxDelay {
		backoff.currentDelay = backoff.maxDelay
	}

	km.logger.Info("Increased rejoin backoff for %s to %v", channel, backoff.currentDelay)
}

// ResetBackoff resets the backoff delay for a channel after successful rejoin
func (km *KickManager) ResetBackoff(channel string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return
	}

	backoff.currentDelay = backoff.minDelay
	backoff.kickCount = 0
	backoff.banned = false

	km.logger.Info("Reset rejoin backoff for %s", channel)
}

// ClearBan marks a channel as no longer banned
func (km *KickManager) ClearBan(channel string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return
	}

	backoff.banned = false
	km.logger.Info("Ban cleared for %s", channel)
}

// GetKickCount returns the number of times the bot has been kicked from a channel
func (km *KickManager) GetKickCount(channel string) int {
	km.mu.RLock()
	defer km.mu.RUnlock()

	backoff, exists := km.channelBackoffs[channel]
	if !exists {
		return 0
	}

	return backoff.kickCount
}

// ScheduleRejoin schedules a rejoin attempt for a channel after the appropriate delay
// Returns a channel that will receive true when it's time to rejoin, or false if banned
func (km *KickManager) ScheduleRejoin(channel string) <-chan bool {
	result := make(chan bool, 1)

	go func() {
		// Check if banned
		if km.IsBanned(channel) {
			result <- false
			return
		}

		// Get the delay
		delay := km.GetRejoinDelay(channel)
		if delay == 0 {
			result <- true
			return
		}

		// Wait for the delay
		time.Sleep(delay)

		// Check again if banned (might have changed during wait)
		if km.IsBanned(channel) {
			result <- false
			return
		}

		result <- true
	}()

	return result
}
