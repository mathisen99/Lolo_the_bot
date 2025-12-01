package irc

import (
	"context"
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/output"
)

// ReconnectionManager handles automatic reconnection with exponential backoff
type ReconnectionManager struct {
	cm                *ConnectionManager
	config            *config.Config
	logger            output.Logger
	currentDelay      time.Duration
	minDelay          time.Duration
	maxDelay          time.Duration
	reconnecting      bool
	stopChan          chan struct{}
	ctx               context.Context
	cancel            context.CancelFunc
	pingTimeoutDetect bool
}

// NewReconnectionManager creates a new reconnection manager
func NewReconnectionManager(cm *ConnectionManager, cfg *config.Config, logger output.Logger) *ReconnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	minDelay := cfg.Limits.GetReconnectDelayMinDuration()
	maxDelay := cfg.Limits.GetReconnectDelayMaxDuration()

	return &ReconnectionManager{
		cm:           cm,
		config:       cfg,
		logger:       logger,
		currentDelay: minDelay,
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		stopChan:     make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins monitoring the connection and handles reconnection
func (rm *ReconnectionManager) Start() {
	go rm.monitorConnection()
}

// Stop stops the reconnection manager
func (rm *ReconnectionManager) Stop() {
	rm.cancel()
	close(rm.stopChan)
}

// monitorConnection monitors the connection and triggers reconnection when needed
func (rm *ReconnectionManager) monitorConnection() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-rm.stopChan:
			return
		case <-ticker.C:
			// Check if we're connected
			if !rm.cm.IsConnected() && !rm.reconnecting {
				rm.logger.Warning("Connection lost, initiating reconnection...")
				go rm.reconnect()
			}
		}
	}
}

// reconnect attempts to reconnect with exponential backoff
func (rm *ReconnectionManager) reconnect() {
	rm.reconnecting = true
	defer func() {
		rm.reconnecting = false
	}()

	attempt := 1

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-rm.stopChan:
			return
		default:
			rm.logger.Info("Reconnection attempt %d (waiting %v)...", attempt, rm.currentDelay)

			// Wait for the current delay
			select {
			case <-time.After(rm.currentDelay):
			case <-rm.ctx.Done():
				return
			case <-rm.stopChan:
				return
			}

			// Attempt to connect
			err := rm.attemptConnection()
			if err == nil {
				// Connection successful
				rm.logger.Success("Reconnection successful!")
				rm.resetBackoff()

				// Rejoin all previously joined channels
				err = rm.cm.RejoinChannels()
				if err != nil {
					rm.logger.Error("Failed to rejoin channels: %v", err)
				}

				return
			}

			// Connection failed, increase backoff
			rm.logger.Error("Reconnection attempt %d failed: %v", attempt, err)
			rm.increaseBackoff()
			attempt++
		}
	}
}

// attemptConnection tries to establish a connection
func (rm *ReconnectionManager) attemptConnection() error {
	// Disconnect any existing connection
	_ = rm.cm.Disconnect()

	// Wait a moment before connecting
	time.Sleep(500 * time.Millisecond)

	// Attempt to connect
	err := rm.cm.Connect()
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	return nil
}

// increaseBackoff doubles the current delay up to the maximum
func (rm *ReconnectionManager) increaseBackoff() {
	rm.currentDelay = rm.currentDelay * 2
	if rm.currentDelay > rm.maxDelay {
		rm.currentDelay = rm.maxDelay
	}
	rm.logger.Info("Backoff increased to %v", rm.currentDelay)
}

// resetBackoff resets the delay to the minimum value
func (rm *ReconnectionManager) resetBackoff() {
	rm.currentDelay = rm.minDelay
	rm.logger.Info("Backoff reset to %v", rm.currentDelay)
}

// TriggerReconnect manually triggers a reconnection
func (rm *ReconnectionManager) TriggerReconnect(reason string) {
	if rm.reconnecting {
		rm.logger.Warning("Reconnection already in progress")
		return
	}

	rm.logger.Warning("Triggering reconnection: %s", reason)
	go rm.reconnect()
}

// OnPingTimeout is called when a ping timeout is detected
func (rm *ReconnectionManager) OnPingTimeout() {
	rm.logger.Error("Ping timeout detected")
	rm.pingTimeoutDetect = true
	rm.TriggerReconnect("ping timeout")
}

// IsReconnecting returns whether a reconnection is in progress
func (rm *ReconnectionManager) IsReconnecting() bool {
	return rm.reconnecting
}
