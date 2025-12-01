package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// Handler manages graceful shutdown of the bot
type Handler struct {
	logger        output.Logger
	shutdownFuncs []func() error
	mu            sync.Mutex
	shutdownChan  chan struct{}
	signalChan    chan os.Signal
	forceTimeout  time.Duration
	shutdownOnce  sync.Once
}

// NewHandler creates a new shutdown handler
func NewHandler(logger output.Logger, forceTimeout time.Duration) *Handler {
	h := &Handler{
		logger:       logger,
		shutdownChan: make(chan struct{}),
		signalChan:   make(chan os.Signal, 1),
		forceTimeout: forceTimeout,
	}

	// Register signal handlers for SIGINT and SIGTERM
	signal.Notify(h.signalChan, syscall.SIGINT, syscall.SIGTERM)

	return h
}

// RegisterShutdownFunc registers a function to be called during shutdown
// Functions are called in the order they were registered
func (h *Handler) RegisterShutdownFunc(fn func() error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shutdownFuncs = append(h.shutdownFuncs, fn)
}

// WaitForShutdown blocks until a shutdown signal is received
func (h *Handler) WaitForShutdown() {
	sig := <-h.signalChan
	h.logger.Info("Received signal: %v", sig)
	h.Shutdown()
}

// Shutdown initiates the graceful shutdown process
func (h *Handler) Shutdown() {
	h.shutdownOnce.Do(func() {
		h.logger.Info("Initiating graceful shutdown...")

		// Create a context with timeout for forced shutdown
		ctx, cancel := context.WithTimeout(context.Background(), h.forceTimeout)
		defer cancel()

		// Create a channel to signal completion
		done := make(chan struct{})

		// Run shutdown functions in a goroutine
		go func() {
			h.executeShutdownFuncs()
			close(done)
		}()

		// Wait for either completion or timeout
		select {
		case <-done:
			h.logger.Success("Graceful shutdown completed")
		case <-ctx.Done():
			h.logger.Warning("Forced shutdown after timeout")
		}

		// Close the shutdown channel to signal completion
		close(h.shutdownChan)
	})
}

// executeShutdownFuncs executes all registered shutdown functions
func (h *Handler) executeShutdownFuncs() {
	h.mu.Lock()
	funcs := make([]func() error, len(h.shutdownFuncs))
	copy(funcs, h.shutdownFuncs)
	h.mu.Unlock()

	for i, fn := range funcs {
		if err := fn(); err != nil {
			h.logger.Error("Shutdown function %d failed: %v", i+1, err)
		}
	}
}

// Done returns a channel that is closed when shutdown is complete
func (h *Handler) Done() <-chan struct{} {
	return h.shutdownChan
}

// Stop stops listening for signals (useful for testing)
func (h *Handler) Stop() {
	signal.Stop(h.signalChan)
	close(h.signalChan)
}
