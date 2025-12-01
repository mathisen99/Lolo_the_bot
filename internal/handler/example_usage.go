package handler

// This file contains example usage of the MessageHandler
// It is not compiled into the binary but serves as documentation

/*
Example: Setting up and using the MessageHandler in the main bot

package main

import (
	"context"
	"time"

	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/handler"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/user"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config/bot.toml")
	if err != nil {
		panic(err)
	}

	// Initialize database
	db, err := database.New("data/bot.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Create user manager
	userMgr := user.NewManager(db)

	// Create logger
	logger := output.NewColorLogger()

	// Create command registry and register core commands
	registry := commands.NewRegistry()

	// Register core commands (verify, user, kick, ban, etc.)
	// ... register commands here ...

	// Create command dispatcher
	dispatcher := commands.NewDispatcher(registry, userMgr, cfg.Bot.CommandPrefix)

	// Create API client
	apiClient := handler.NewAPIClient(
		cfg.Bot.APIEndpoint,
		time.Duration(cfg.Bot.APITimeout)*time.Second,
	)

	// Create message splitter
	msgSplitter := splitter.New(cfg.Server.MaxMessageLength)

	// Create message handler
	handlerConfig := &handler.MessageHandlerConfig{
		Dispatcher:    dispatcher,
		APIClient:     apiClient,
		UserManager:   userMgr,
		DB:            db,
		Logger:        logger,
		Splitter:      msgSplitter,
		BotNick:       cfg.Server.Nickname,
		CommandPrefix: cfg.Bot.CommandPrefix,
		TestMode:      cfg.Bot.TestMode,
	}

	messageHandler := handler.NewMessageHandler(handlerConfig)

	// Connect to IRC server
	// ... IRC connection code ...

	// Handle incoming PRIVMSG events
	ircClient.AddCallback("PRIVMSG", func(event *irc.Event) {
		// Extract message details
		nick := event.Nick
		hostmask := event.User + "@" + event.Host
		channel := event.Params[0]
		message := event.Message()

		// Determine if this is a PM
		isPM := channel == cfg.Server.Nickname

		// If PM, set channel to empty
		if isPM {
			channel = ""
		}

		// Process the message
		ctx := context.Background()
		responses, err := messageHandler.HandleMessage(
			ctx,
			nick,
			hostmask,
			channel,
			message,
			isPM,
		)

		if err != nil {
			logger.Error("Failed to handle message: %v", err)
			return
		}

		// Send responses back to IRC
		target := channel
		if isPM {
			target = nick
		}

		for _, response := range responses {
			ircClient.SendMessage(target, response)
		}
	})

	// Handle nickname changes
	ircClient.AddCallback("NICK", func(event *irc.Event) {
		// If the bot's nickname changed, update the handler
		if event.Nick == ircClient.GetNick() {
			newNick := event.Message()
			messageHandler.UpdateBotNick(newNick)
			logger.Info("Bot nickname changed to: %s", newNick)
		}
	})

	// Start the bot
	// ... IRC connection and main loop ...
}

Example: Processing different types of messages

// Core command (executed in Go)
responses, err := messageHandler.HandleMessage(
	ctx,
	"alice",
	"alice@host",
	"#channel",
	"!help",
	false,
)
// Returns: ["Available commands: help, ping, status, ..."]

// API command (routed to Python)
responses, err := messageHandler.HandleMessage(
	ctx,
	"bob",
	"bob@host",
	"#channel",
	"!weather Boston",
	false,
)
// Sends to Python API, returns API response

// Bot mention
responses, err := messageHandler.HandleMessage(
	ctx,
	"charlie",
	"charlie@host",
	"#channel",
	"Hey Lolo, what's up?",
	false,
)
// Sends to Python API mention handler, returns response

// Regular message (no response)
responses, err := messageHandler.HandleMessage(
	ctx,
	"dave",
	"dave@host",
	"#channel",
	"Just chatting with friends",
	false,
)
// Returns: nil (no response needed)

// Private message command
responses, err := messageHandler.HandleMessage(
	ctx,
	"eve",
	"eve@host",
	"",
	"!verify mypassword",
	true,
)
// Processes verify command in PM

Example: Handling ignored users and disabled channels

// Add an ignored user
userMgr.AddUser("spammer", "spam@host", database.LevelIgnored)

// Ignored user's commands are filtered out
responses, err := messageHandler.HandleMessage(
	ctx,
	"spammer",
	"spam@host",
	"#channel",
	"!help",
	false,
)
// Returns: nil (no response for ignored user)

// Disable a channel
db.SetChannelState("#offtopic", false)

// Commands in disabled channels are filtered out
responses, err := messageHandler.HandleMessage(
	ctx,
	"alice",
	"alice@host",
	"#offtopic",
	"!help",
	false,
)
// Returns: nil (no response for disabled channel)

Example: Message splitting for long responses

// Long message that exceeds IRC limits
longMessage := strings.Repeat("This is a very long message. ", 50)

// Handler automatically splits it
responses, err := messageHandler.HandleMessage(
	ctx,
	"alice",
	"alice@host",
	"#channel",
	"!longcommand",
	false,
)
// Returns: ["part1...", "part2...", "part3..."]
// Each part is <= 400 bytes and doesn't break words

Example: Test mode for development

// Enable test mode
handlerConfig.TestMode = true
messageHandler := handler.NewMessageHandler(handlerConfig)

// API commands return mock responses
responses, err := messageHandler.HandleMessage(
	ctx,
	"alice",
	"alice@host",
	"#channel",
	"!weather Boston",
	false,
)
// Returns: "Test mode: Command 'weather' with args [Boston]"
// No actual API call made

Example: Updating bot nickname on collision

// Bot nickname collision detected
ircClient.AddCallback("433", func(event *irc.Event) {
	// Try alternative nickname
	newNick := cfg.Server.AltNicknames[0]
	ircClient.Nick(newNick)

	// Update message handler
	messageHandler.UpdateBotNick(newNick)
	logger.Warning("Nickname collision, using: %s", newNick)
})

Example: Error handling

responses, err := messageHandler.HandleMessage(
	ctx,
	"alice",
	"alice@host",
	"#channel",
	"!help",
	false,
)

if err != nil {
	// Log error
	logger.Error("Message handling failed: %v", err)

	// Optionally send error message to user
	ircClient.SendMessage("#channel", "An error occurred processing your message")
}

*/
