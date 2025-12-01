package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
	refillWindow time.Duration
}

// NewTokenBucket creates a new token bucket rate limiter
// messagesPerWindow: number of messages allowed per window
// windowSeconds: time window in seconds
func NewTokenBucket(messagesPerWindow int, windowSeconds int) *TokenBucket {
	maxTokens := float64(messagesPerWindow)
	refillRate := maxTokens / float64(windowSeconds)

	return &TokenBucket{
		tokens:       maxTokens,
		maxTokens:    maxTokens,
		refillRate:   refillRate,
		lastRefill:   time.Now(),
		refillWindow: time.Duration(windowSeconds) * time.Second,
	}
}

// Allow checks if a message can be sent immediately
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// Wait returns the duration to wait before the next message can be sent
func (tb *TokenBucket) Wait() time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1.0 {
		return 0
	}

	// Calculate time needed to accumulate 1 token
	tokensNeeded := 1.0 - tb.tokens
	waitTime := time.Duration(tokensNeeded/tb.refillRate*1000) * time.Millisecond

	return waitTime
}

// refill adds tokens based on elapsed time since last refill
// Must be called with mutex locked
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Calculate tokens to add based on elapsed time
	tokensToAdd := elapsed.Seconds() * tb.refillRate

	tb.tokens += tokensToAdd
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	tb.lastRefill = now
}

// Reset resets the token bucket to full capacity
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}
