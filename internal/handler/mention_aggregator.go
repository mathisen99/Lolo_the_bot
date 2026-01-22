package handler

import (
	"context"
	"sync"
	"time"
)

// MentionAggregator buffers mention messages to handle IRC message overflow.
// When a user sends a long message that gets split by their IRC client,
// this aggregator waits briefly to collect all parts before processing.
type MentionAggregator struct {
	mu              sync.Mutex
	pendingMentions map[string]*pendingMention // key: "channel:nick"
	aggregateDelay  time.Duration              // How long to wait for overflow messages
}

// pendingMention tracks a mention that's waiting for potential overflow messages
type pendingMention struct {
	nick           string
	hostmask       string
	channel        string
	messages       []string  // All message parts collected
	firstMessageAt time.Time // When the first message arrived
	timer          *time.Timer
	cancelFunc     context.CancelFunc
	callback       MentionCallback
	statusCallback func(string)
}

// MentionCallback is called when a mention is ready to be processed
type MentionCallback func(ctx context.Context, nick, hostmask, channel, fullMessage string, statusCallback func(string)) ([]string, error)

// NewMentionAggregator creates a new mention aggregator
func NewMentionAggregator(aggregateDelay time.Duration) *MentionAggregator {
	return &MentionAggregator{
		pendingMentions: make(map[string]*pendingMention),
		aggregateDelay:  aggregateDelay,
	}
}

// pendingKey generates a unique key for a nick+channel combination
func pendingKey(channel, nick string) string {
	return channel + ":" + nick
}

// AddMention adds a mention message to the aggregator.
// If this is the first message from this nick in this channel, it starts a timer.
// If there's already a pending mention, it appends this message and resets the timer.
// When the timer fires, the callback is invoked with the full aggregated message.
func (a *MentionAggregator) AddMention(
	ctx context.Context,
	nick, hostmask, channel, message string,
	callback MentionCallback,
	statusCallback func(string),
) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := pendingKey(channel, nick)

	if pending, exists := a.pendingMentions[key]; exists {
		// There's already a pending mention from this nick - this is an overflow message
		pending.messages = append(pending.messages, message)

		// Reset the timer to wait for more potential overflow
		pending.timer.Reset(a.aggregateDelay)
		return
	}

	// Use a background context for the async callback since the original
	// request context will be cancelled when HandleMessage returns.
	// We create our own cancellable context for shutdown purposes.
	mentionCtx, cancel := context.WithCancel(context.Background())

	pending := &pendingMention{
		nick:           nick,
		hostmask:       hostmask,
		channel:        channel,
		messages:       []string{message},
		firstMessageAt: time.Now(),
		cancelFunc:     cancel,
		callback:       callback,
		statusCallback: statusCallback,
	}

	// Create timer that will fire after the aggregate delay
	pending.timer = time.AfterFunc(a.aggregateDelay, func() {
		a.processPendingMention(mentionCtx, key)
	})

	a.pendingMentions[key] = pending
}

// AddFollowUpMessage adds a follow-up message from the same nick in the same channel.
// This is called for messages that don't contain a mention but might be overflow.
// Returns true if the message was added to a pending mention, false otherwise.
func (a *MentionAggregator) AddFollowUpMessage(channel, nick, message string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := pendingKey(channel, nick)

	pending, exists := a.pendingMentions[key]
	if !exists {
		return false
	}

	// Check if this message arrived within a reasonable time window
	// (should be very quick for IRC overflow, typically < 100ms)
	if time.Since(pending.firstMessageAt) > 2*time.Second {
		// Too much time has passed, this is probably a new message, not overflow
		return false
	}

	// Add the message and reset the timer
	pending.messages = append(pending.messages, message)
	pending.timer.Reset(a.aggregateDelay)

	return true
}

// processPendingMention is called when the timer fires - processes the aggregated mention
func (a *MentionAggregator) processPendingMention(ctx context.Context, key string) {
	a.mu.Lock()
	pending, exists := a.pendingMentions[key]
	if !exists {
		a.mu.Unlock()
		return
	}

	// Remove from pending map
	delete(a.pendingMentions, key)
	a.mu.Unlock()

	// Check if context was cancelled
	if ctx.Err() != nil {
		return
	}

	// Combine all messages into one
	fullMessage := combineMessages(pending.messages)

	// Call the callback with the full message
	// This runs in the timer goroutine, which is fine since HandleMention
	// already runs in a goroutine from the IRC handler
	_, _ = pending.callback(ctx, pending.nick, pending.hostmask, pending.channel, fullMessage, pending.statusCallback)
}

// combineMessages joins multiple message parts into a single message
func combineMessages(messages []string) string {
	if len(messages) == 1 {
		return messages[0]
	}

	// Join with space - IRC overflow messages are typically split mid-sentence
	result := messages[0]
	for i := 1; i < len(messages); i++ {
		result += " " + messages[i]
	}
	return result
}

// HasPendingMention checks if there's a pending mention for a nick in a channel
func (a *MentionAggregator) HasPendingMention(channel, nick string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, exists := a.pendingMentions[pendingKey(channel, nick)]
	return exists
}

// CancelPending cancels any pending mention for a nick in a channel
func (a *MentionAggregator) CancelPending(channel, nick string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := pendingKey(channel, nick)
	if pending, exists := a.pendingMentions[key]; exists {
		pending.timer.Stop()
		pending.cancelFunc()
		delete(a.pendingMentions, key)
	}
}

// Shutdown cancels all pending mentions
func (a *MentionAggregator) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key, pending := range a.pendingMentions {
		pending.timer.Stop()
		pending.cancelFunc()
		delete(a.pendingMentions, key)
	}
}
