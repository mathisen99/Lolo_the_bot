package ratelimit

import (
	"context"
	"time"
)

// RateLimiter provides rate limiting, message queuing, and cooldown tracking
type RateLimiter struct {
	bucket   *TokenBucket
	queue    *MessageQueue
	cooldown *CooldownTracker
}

// New creates a new RateLimiter with the specified configuration
func New(messagesPerWindow, windowSeconds, maxQueueSize int, cooldownDuration time.Duration, sendFunc func(target, message string) error) *RateLimiter {
	bucket := NewTokenBucket(messagesPerWindow, windowSeconds)
	queue := NewMessageQueue(maxQueueSize, bucket, sendFunc)
	cooldown := NewCooldownTracker(cooldownDuration)

	return &RateLimiter{
		bucket:   bucket,
		queue:    queue,
		cooldown: cooldown,
	}
}

// Allow checks if a message can be sent immediately
func (rl *RateLimiter) Allow() bool {
	return rl.bucket.Allow()
}

// Wait returns the duration to wait before the next message can be sent
func (rl *RateLimiter) Wait() time.Duration {
	return rl.bucket.Wait()
}

// QueueMessage adds a message to the queue
func (rl *RateLimiter) QueueMessage(target, message string) error {
	rl.queue.Enqueue(target, message)
	return nil
}

// QueueSize returns the current queue size
func (rl *RateLimiter) QueueSize() int {
	return rl.queue.Size()
}

// DroppedMessages returns the number of dropped messages
func (rl *RateLimiter) DroppedMessages() int {
	return rl.queue.DroppedCount()
}

// CheckCooldown checks if a user can execute a command
func (rl *RateLimiter) CheckCooldown(user, command string) (bool, time.Duration) {
	return rl.cooldown.Check(user, command)
}

// SetCooldown sets a cooldown for a user-command combination
func (rl *RateLimiter) SetCooldown(user, command string, duration time.Duration) {
	if duration == 0 {
		rl.cooldown.Set(user, command)
	} else {
		rl.cooldown.SetWithDuration(user, command, duration)
	}
}

// ClearCooldown removes a cooldown for a user-command combination
func (rl *RateLimiter) ClearCooldown(user, command string) {
	rl.cooldown.Clear(user, command)
}

// Start begins processing the message queue
func (rl *RateLimiter) Start(ctx context.Context) {
	rl.queue.Start(ctx)

	// Start cooldown cleanup goroutine
	stopCh := make(chan struct{})
	go rl.cooldown.StartCleanup(5*time.Minute, stopCh)

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()
}

// Stop stops the message queue processor
func (rl *RateLimiter) Stop() {
	rl.queue.Stop()
}

// Reset resets the token bucket to full capacity
func (rl *RateLimiter) Reset() {
	rl.bucket.Reset()
}
