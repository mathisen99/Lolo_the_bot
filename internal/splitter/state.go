package splitter

import (
	"sync"
	"time"
)

// SplitState tracks the state of a message split operation
type SplitState struct {
	// Unique identifier for this split operation
	ID string

	// Original message that was split
	OriginalMessage string

	// All parts of the split message
	Parts []string

	// Index of the next part to send (0-based)
	NextPartIndex int

	// Target (channel or nick) to send to
	Target string

	// Timestamp when the split started
	StartTime time.Time

	// Timestamp of the last part sent successfully
	LastSuccessTime time.Time

	// Number of parts sent successfully
	PartsSent int

	// Error message if split failed
	Error string

	// Whether the split is complete
	Complete bool
}

// StateTracker tracks in-flight message splits for error recovery
type StateTracker struct {
	mu     sync.RWMutex
	states map[string]*SplitState
}

// NewStateTracker creates a new split state tracker
func NewStateTracker() *StateTracker {
	return &StateTracker{
		states: make(map[string]*SplitState),
	}
}

// StartSplit creates a new split state for tracking
func (st *StateTracker) StartSplit(id, target, originalMessage string, parts []string) *SplitState {
	st.mu.Lock()
	defer st.mu.Unlock()

	state := &SplitState{
		ID:              id,
		OriginalMessage: originalMessage,
		Parts:           parts,
		NextPartIndex:   0,
		Target:          target,
		StartTime:       time.Now(),
		PartsSent:       0,
		Complete:        false,
	}

	st.states[id] = state
	return state
}

// GetState retrieves the current state of a split operation
func (st *StateTracker) GetState(id string) *SplitState {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return st.states[id]
}

// MarkPartSent marks a part as successfully sent
func (st *StateTracker) MarkPartSent(id string, partIndex int) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	state, ok := st.states[id]
	if !ok {
		return ErrSplitNotFound
	}

	if partIndex != state.NextPartIndex {
		return ErrInvalidPartIndex
	}

	state.PartsSent++
	state.NextPartIndex++
	state.LastSuccessTime = time.Now()

	// Check if all parts have been sent
	if state.NextPartIndex >= len(state.Parts) {
		state.Complete = true
	}

	return nil
}

// MarkSplitFailed marks a split as failed with an error message
func (st *StateTracker) MarkSplitFailed(id string, errMsg string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	state, ok := st.states[id]
	if !ok {
		return ErrSplitNotFound
	}

	state.Error = errMsg
	state.Complete = true

	return nil
}

// CompleteSplit marks a split as complete and removes it from tracking
func (st *StateTracker) CompleteSplit(id string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	state, ok := st.states[id]
	if !ok {
		return ErrSplitNotFound
	}

	state.Complete = true
	delete(st.states, id)

	return nil
}

// GetIncompleteSplits returns all incomplete splits (for recovery on restart)
func (st *StateTracker) GetIncompleteSplits() []*SplitState {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var incomplete []*SplitState
	for _, state := range st.states {
		if !state.Complete {
			incomplete = append(incomplete, state)
		}
	}

	return incomplete
}

// CleanupOldSplits removes splits that have been in progress for too long
// Returns the number of splits cleaned up
func (st *StateTracker) CleanupOldSplits(maxAge time.Duration) int {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for id, state := range st.states {
		if now.Sub(state.StartTime) > maxAge {
			delete(st.states, id)
			cleaned++
		}
	}

	return cleaned
}

// GetStats returns statistics about tracked splits
func (st *StateTracker) GetStats() (total, complete, incomplete int) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	total = len(st.states)
	for _, state := range st.states {
		if state.Complete {
			complete++
		} else {
			incomplete++
		}
	}

	return
}
