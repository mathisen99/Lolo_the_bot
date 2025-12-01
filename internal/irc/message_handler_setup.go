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

			// Process the message with timeout (2 minutes for image generation)
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			responses, err := messageHandler.HandleMessage(
				ctx,
				nick,
				hostmask,
				handlerChannel,
				message,
				isPM,
			)

			if err != nil {
				logger.Error("Failed to handle message from %s: %v", nick, err)
				return
			}

			// Send responses back to IRC
			target := channel
			if isPM {
				target = nick
			}

			for _, response := range responses {
				if err := cm.client.SendMessage(target, response); err != nil {
					logger.Error("Failed to send message to %s: %v", target, err)
				}
			}
		}()
	})
}
