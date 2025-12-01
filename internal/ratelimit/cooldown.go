package ratelimit

import (
	"sync"
	"time"
)

// CooldownKey represents a unique user-command combination
type CooldownKey struct {
	User    string
	Command string
}

// CooldownTracker tracks per-user command cooldowns
type CooldownTracker struct {
	mu        sync.RWMutex
	cooldowns map[CooldownKey]time.Time
	duration  time.Duration
}

// NewCooldownTracker creates a new cooldown tracker
func NewCooldownTracker(cooldownDuration time.Duration) *CooldownTracker {
	return &CooldownTracker{
		cooldowns: make(map[CooldownKey]time.Time),
		duration:  cooldownDuration,
	}
}

// Check checks if a user can execute a command
// Returns true if allowed, false if on cooldown
// Also returns the remaining cooldown duration
func (ct *CooldownTracker) Check(user, command string) (bool, time.Duration) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	key := CooldownKey{User: user, Command: command}
	lastUsed, exists := ct.cooldowns[key]

	if !exists {
		return true, 0
	}

	elapsed := time.Since(lastUsed)
	if elapsed >= ct.duration {
		return true, 0
	}

	remaining := ct.duration - elapsed
	return false, remaining
}

// Set sets a cooldown for a user-command combination
func (ct *CooldownTracker) Set(user, command string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := CooldownKey{User: user, Command: command}
	ct.cooldowns[key] = time.Now()
}

// SetWithDuration sets a cooldown with a custom duration
func (ct *CooldownTracker) SetWithDuration(user, command string, duration time.Duration) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := CooldownKey{User: user, Command: command}
	// Set the time in the past so that the cooldown expires at the right time
	ct.cooldowns[key] = time.Now().Add(-ct.duration).Add(duration)
}

// Clear removes a cooldown for a user-command combination
func (ct *CooldownTracker) Clear(user, command string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := CooldownKey{User: user, Command: command}
	delete(ct.cooldowns, key)
}

// Cleanup removes expired cooldowns to prevent memory leaks
func (ct *CooldownTracker) Cleanup() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	now := time.Now()
	for key, lastUsed := range ct.cooldowns {
		if now.Sub(lastUsed) >= ct.duration {
			delete(ct.cooldowns, key)
		}
	}
}

// StartCleanup starts a goroutine that periodically cleans up expired cooldowns
func (ct *CooldownTracker) StartCleanup(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			ct.Cleanup()
		}
	}
}
