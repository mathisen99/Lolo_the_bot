package irc

import (
	"context"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/handler"
	"github.com/yourusername/lolo/internal/output"
)

// SetupBotMessageHandler configures the connection manager to process bot commands and messages
// This must be called before Connect()
func (cm *ConnectionManager) SetupBotMessageHandler(messageHandler *handler.MessageHandler, cfg *config.Config, logger output.Logger) {
	// Set up the send message callback for the mention aggregator
	// This allows asynchronous mention responses to be sent back to IRC
	messageHandler.SetSendMessageFunc(func(target, message string) error {
		return cm.client.SendMessage(target, message)
	})

	// Set up the PRIVMSG handler callback
	cm.SetPrivMsgHandler(func(nick, hostmask, channel, message string, isPM bool) {
		// Process messages in a goroutine to avoid blocking the IRC event loop
		// This ensures PING/PONG handling continues during long-running operations
		go func() {
			// If PM, use empty string for channel in handler
			handlerChannel := channel
			if isPM {
				handlerChannel = ""
			}

			// Process the message with timeout
			// Use 8 minutes to accommodate --deep mode which does thorough research
			// Normal requests complete much faster, deep mode is rate-limited (3/day)
			ctx, cancel := context.WithTimeout(context.Background(), 480*time.Second)
			defer cancel()

			// Send responses back to IRC
			target := channel
			if isPM {
				target = nick
			}

			// Callback for streaming status updates
			statusCallback := func(msg string) {
				// Prepend a status indicator or color if desired, e.g. "[Status] Reading paper..."
				// For now, send raw message as the AI formats it.
				if err := cm.client.SendMessage(target, msg); err != nil {
					logger.Error("Failed to send status update to %s: %v", target, err)
				}
			}

			responses, err := messageHandler.HandleMessage(
				ctx,
				nick,
				hostmask,
				handlerChannel,
				message,
				isPM,
				statusCallback,
			)

			if err != nil {
				logger.Error("Failed to handle message from %s: %v", nick, err)
				return
			}

			// Send final responses back to IRC
			// Note: For mentions, responses may be nil because the aggregator
			// handles sending them asynchronously after collecting overflow messages
			for _, response := range responses {
				if err := cm.client.SendMessage(target, response); err != nil {
					logger.Error("Failed to send message to %s: %v", target, err)
				}
			}
		}()
	})
}
