package ratelimit

import (
	"context"
	"sync"
	"time"
)

// QueuedMessage represents a message waiting to be sent
type QueuedMessage struct {
	Target  string // Channel or nick to send to
	Message string // Message content
}

// MessageQueue implements a FIFO message queue with overflow handling
type MessageQueue struct {
	mu          sync.Mutex
	queue       []QueuedMessage
	maxSize     int
	limiter     *TokenBucket
	sendFunc    func(target, message string) error
	stopCh      chan struct{}
	stoppedCh   chan struct{}
	droppedMsgs int
}

// NewMessageQueue creates a new message queue
func NewMessageQueue(maxSize int, limiter *TokenBucket, sendFunc func(target, message string) error) *MessageQueue {
	return &MessageQueue{
		queue:     make([]QueuedMessage, 0, maxSize),
		maxSize:   maxSize,
		limiter:   limiter,
		sendFunc:  sendFunc,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Enqueue adds a message to the queue
// If the queue is full, drops the oldest message
func (mq *MessageQueue) Enqueue(target, message string) bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	msg := QueuedMessage{
		Target:  target,
		Message: message,
	}

	// Check if queue is full
	if len(mq.queue) >= mq.maxSize {
		// Drop oldest message (first in queue)
		mq.queue = mq.queue[1:]
		mq.droppedMsgs++
	}

	// Add new message to end of queue
	mq.queue = append(mq.queue, msg)

	return true
}

// Dequeue removes and returns the next message from the queue
func (mq *MessageQueue) Dequeue() (QueuedMessage, bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.queue) == 0 {
		return QueuedMessage{}, false
	}

	// Get first message (FIFO)
	msg := mq.queue[0]
	mq.queue = mq.queue[1:]

	return msg, true
}

// Size returns the current queue size
func (mq *MessageQueue) Size() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	return len(mq.queue)
}

// DroppedCount returns the number of dropped messages
func (mq *MessageQueue) DroppedCount() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	return mq.droppedMsgs
}

// Start begins processing the queue in a goroutine
func (mq *MessageQueue) Start(ctx context.Context) {
	go mq.processQueue(ctx)
}

// Stop stops the queue processor
func (mq *MessageQueue) Stop() {
	close(mq.stopCh)
	<-mq.stoppedCh
}

// processQueue continuously processes messages from the queue
func (mq *MessageQueue) processQueue(ctx context.Context) {
	defer close(mq.stoppedCh)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mq.stopCh:
			return
		case <-ticker.C:
			mq.processNextMessage()
		}
	}
}

// processNextMessage attempts to send the next message if rate limit allows
func (mq *MessageQueue) processNextMessage() {
	// Check if we can send
	if !mq.limiter.Allow() {
		return
	}

	// Get next message
	msg, ok := mq.Dequeue()
	if !ok {
		return
	}

	// Send the message
	if err := mq.sendFunc(msg.Target, msg.Message); err != nil {
		// If send fails, re-queue the message at the front
		mq.mu.Lock()
		mq.queue = append([]QueuedMessage{msg}, mq.queue...)
		mq.mu.Unlock()
	}
}
