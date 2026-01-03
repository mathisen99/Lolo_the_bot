package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/lolo/internal/callback"
	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/handler"
	"github.com/yourusername/lolo/internal/irc"
	"github.com/yourusername/lolo/internal/maintenance"
	"github.com/yourusername/lolo/internal/mockapi"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/shutdown"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/user"
)

func main() {
	// Parse command-line flags
	rollbackFlag := flag.Bool("rollback", false, "Rollback the last applied database migration")
	flag.Parse()

	// Create logger first for colored output
	logger := output.NewColorLogger()
	logger.Info("Lolo IRC Bot - Starting...")

	// Load or create configuration first to check test mode
	cfg, err := config.LoadOrCreate("config/bot.toml")
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}
	logger.Success("Configuration loaded")

	// Initialize database first (needed for rollback)
	// Use test database if test mode is enabled
	var db *database.DB
	if cfg.Bot.TestMode {
		logger.Info("Test mode enabled - using in-memory test database")
		db, err = database.NewTest()
	} else {
		db, err = database.New("data/bot.db")
	}
	if err != nil {
		logger.Error("Failed to initialize database: %v", err)
		os.Exit(1)
	}
	logger.Success("Database initialized")

	// Handle rollback if requested
	if *rollbackFlag {
		logger.Info("Rolling back last migration...")
		if err := db.Rollback(); err != nil {
			logger.Error("Rollback failed: %v", err)
			_ = db.Close()
			os.Exit(1)
		}
		logger.Success("Migration rolled back successfully")
		_ = db.Close()
		os.Exit(0)
	}

	// Initialize output with error logging
	out, err := output.NewOutput("data/error.log")
	if err != nil {
		logger.Error("Failed to initialize output: %v", err)
		_ = db.Close()
		os.Exit(1)
	}
	logger.Success("Output and error logging initialized")

	// Start database maintenance scheduler (Requirement 28.1-28.5, 14.6-14.7)
	maintenanceScheduler := maintenance.New(
		db.Conn(),
		logger,
		cfg.Database.GetVacuumIntervalDuration(),
		cfg.Database.MessageRetentionDays,
	)
	if err := maintenanceScheduler.Start(); err != nil {
		logger.Error("Failed to start maintenance scheduler: %v", err)
		os.Exit(1)
	}
	logger.Success("Database maintenance scheduler started")

	// Create user manager
	userMgr := user.NewManager(db)

	// Check if owner password needs to be set (Requirement 11.2)
	hasPassword, err := userMgr.HasOwnerPassword()
	if err != nil {
		logger.Error("Failed to check owner password: %v", err)
		_ = db.Close()
		os.Exit(1)
	}

	if !hasPassword {
		logger.Info("No owner password set. Please enter an admin password for owner verification:")
		logger.Info("This password will be used with the !verify command via PM to become owner.")
		fmt.Print("Enter admin password: ")

		var password string
		_, err := fmt.Scanln(&password)
		if err != nil || password == "" {
			logger.Error("Failed to read password or password is empty")
			_ = db.Close()
			os.Exit(1)
		}

		// Set the owner password (hashed with bcrypt)
		if err := userMgr.SetOwnerPassword(password); err != nil {
			logger.Error("Failed to set owner password: %v", err)
			_ = db.Close()
			os.Exit(1)
		}

		logger.Success("Owner password set successfully!")
		logger.Info("To become owner, send this command via PM (NOT in channel):")
		logger.Info("  /msg %s !verify %s", cfg.Server.Nickname, password)
	}

	// Create command registry and dispatcher
	registry := commands.NewRegistry()
	dispatcher := commands.NewDispatcher(registry, userMgr, cfg.Bot.CommandPrefix)

	// Capture start time for metrics and uptime tracking
	startTime := time.Now()

	// Create API client (or mock client if in test mode)
	var apiClient handler.APIClientInterface
	if cfg.Bot.TestMode {
		logger.Info("Test mode enabled - using mock API client")
		apiClient = mockapi.New()
	} else {
		apiClient = handler.NewAPIClient(
			cfg.Bot.APIEndpoint,
			time.Duration(cfg.Bot.APITimeout)*time.Second,
		)
	}

	// Create API health checker adapter for info commands
	apiHealthChecker := newAPIHealthCheckerAdapter(apiClient)

	// Create IRC connection manager (needed for commands that interact with IRC)
	connManager := irc.NewConnectionManager(cfg, logger, db, userMgr)

	// Register all core commands (after API client and IRC client are created)
	registerCoreCommands(registry, db, userMgr, logger, startTime, cfg.Bot.APIEndpoint, apiHealthChecker, connManager.GetClient())

	// Check Python API health at startup (Requirement 16.5)
	logger.Info("Checking API health...")
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer healthCancel()

	healthResp, err := apiClient.CheckHealth(healthCtx)
	if err != nil {
		logger.Warning("API health check failed: %v", err)
		logger.Warning("Bot will continue, but API commands may not work")
	} else {
		logger.Success("API is healthy (version: %s, uptime: %.0fs)",
			healthResp.Version, healthResp.Uptime)
	}

	// Create message splitter
	msgSplitter := splitter.New(cfg.Server.MaxMessageLength)

	// Create error handler
	errorHandler := errors.NewErrorHandler(out)

	// Create message handler
	handlerConfig := &handler.MessageHandlerConfig{
		Dispatcher:            dispatcher,
		APIClient:             apiClient,
		UserManager:           userMgr,
		DB:                    db,
		Logger:                logger,
		ErrorHandler:          errorHandler,
		Splitter:              msgSplitter,
		BotNick:               cfg.Server.Nickname,
		CommandPrefix:         cfg.Bot.CommandPrefix,
		TestMode:              cfg.Bot.TestMode,
		ImageDownloadChannels: cfg.Images.DownloadChannels,
	}
	messageHandler := handler.NewMessageHandler(handlerConfig)

	// Start callback server for Python API to call back (IRC command tool)
	var callbackServer *callback.Server
	if cfg.Bot.CallbackPort > 0 {
		callbackServer = callback.NewServer(connManager.GetClient(), logger, cfg.Bot.CallbackPort)
		callbackServer.SetDatabase(db) // Enable bot_status and channel_info commands
		if err := callbackServer.Start(); err != nil {
			logger.Error("Failed to start callback server: %v", err)
		} else {
			logger.Success("Callback server started on port %d", cfg.Bot.CallbackPort)

			// Wire up IRC response handlers to callback server
			connManager.SetNoticeHandler(callbackServer.HandleServiceResponse)
			connManager.SetNumericHandler(callbackServer.HandleNumericResponse)
			connManager.SetCTCPResponseHandler(callbackServer.HandleCTCPResponse)
		}
	}

	// Create and wire up channel user tracker for tracking op status, user counts, etc.
	channelTracker := irc.NewChannelTracker(db, logger, cfg.Server.Nickname)
	connManager.SetChannelUserTracker(channelTracker)

	// Cache command metadata from Python API (Requirement 31.5)
	if healthResp != nil {
		logger.Info("Fetching command metadata from Python API...")
		metadataCtx, metadataCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer metadataCancel()

		if err := messageHandler.CacheCommandMetadata(metadataCtx); err != nil {
			logger.Warning("Failed to cache command metadata: %v", err)
			logger.Warning("Help text and validation may not be available for API commands")
		}
	}

	// Set up shutdown handler with 5-second forced timeout (Requirement 10.5)
	shutdownHandler := shutdown.NewHandler(logger, 5*time.Second)

	// Register shutdown functions in order
	// 1. Send IRC QUIT message (Requirement 10.1)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Sending QUIT message to IRC server...")
		return connManager.Disconnect()
	})

	// 2. Stop reconnection manager
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Stopping reconnection manager...")
		connManager.StopReconnectionManager()
		return nil
	})

	// 3. Stop maintenance scheduler
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Stopping database maintenance scheduler...")
		if err := maintenanceScheduler.Stop(); err != nil {
			logger.Warning("Failed to stop maintenance scheduler: %v", err)
		}
		return nil
	})

	// 3.5. Stop callback server
	if callbackServer != nil {
		shutdownHandler.RegisterShutdownFunc(func() error {
			logger.Info("Stopping callback server...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return callbackServer.Stop(ctx)
		})
	}

	// 4. Wait for in-flight API requests (Requirement 10.3)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Waiting for in-flight API requests...")
		if apiClient.WaitForInflightRequests(3 * time.Second) {
			logger.Success("All API requests completed")
		} else {
			logger.Warning("Some API requests timed out")
		}
		return nil
	})

	// 5. Close database connection (Requirement 10.2)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Closing database connection...")
		if err := db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
		logger.Success("Database connection closed")
		return nil
	})

	// 6. Output colored shutdown message (Requirement 10.4)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Success("Lolo IRC Bot has shut down gracefully. Goodbye!")
		return nil
	})

	// Start shutdown handler in background
	go shutdownHandler.WaitForShutdown()

	// Set up IRC message handler BEFORE connecting
	setupIRCHandlers(connManager, messageHandler, cfg, logger)

	// Start the IRC client event loop in a goroutine
	// This must run BEFORE Connect() because Connect() waits for registration messages
	// which are processed by the event loop
	//
	// IMPORTANT: When Run() returns due to a connection error (e.g., "connection reset by peer"),
	// we should NOT trigger shutdown. Instead, we let the reconnection manager handle it.
	// Only trigger shutdown for intentional disconnects (SIGINT/SIGTERM or !quit command).
	go func() {
		for {
			if err := connManager.Run(); err != nil {
				logger.Error("IRC client error: %v", err)

				// Check if this is a shutdown-initiated disconnect
				// If shutdown is already in progress, don't try to reconnect
				select {
				case <-shutdownHandler.Done():
					// Shutdown already completed, exit the loop
					return
				default:
					// Not a shutdown - this is a connection error
					// Let the reconnection manager handle it
					// Run() will be called again immediately - it internally waits for
					// a new connection to be established (waits for c.conn != nil)
					logger.Info("Connection lost, reconnection manager will handle it...")

					// Small delay to avoid tight loop while reconnection manager works
					time.Sleep(500 * time.Millisecond)

					// Continue the loop to call Run() again
					// Run() will wait for the new connection to be ready
					continue
				}
			}
			// Run() returned without error (clean disconnect)
			// This happens when we intentionally disconnect
			break
		}
	}()

	// Give the event loop a moment to start
	time.Sleep(100 * time.Millisecond)

	// Connect to IRC server (handles authentication and waits for registration)
	if err := connManager.Connect(); err != nil {
		logger.Error("Failed to connect to IRC: %v", err)
		shutdownHandler.Shutdown()
		<-shutdownHandler.Done()
		os.Exit(1)
	}

	// Start reconnection manager for automatic reconnection on disconnect
	connManager.StartReconnectionManager()

	// Join configured channels
	if err := connManager.JoinChannels(); err != nil {
		logger.Error("Failed to join channels: %v", err)
	}

	// Start ping monitor to detect connection issues
	connManager.StartPingMonitor()

	logger.Success("Bot initialization complete. Connected and ready.")

	// Wait for graceful shutdown to complete
	<-shutdownHandler.Done()
}

// apiHealthCheckerAdapter adapts handler.APIClientInterface to commands.APIHealthChecker
type apiHealthCheckerAdapter struct {
	client handler.APIClientInterface
}

func newAPIHealthCheckerAdapter(client handler.APIClientInterface) commands.APIHealthChecker {
	return &apiHealthCheckerAdapter{client: client}
}

func (a *apiHealthCheckerAdapter) CheckHealth(ctx context.Context) error {
	_, err := a.client.CheckHealth(ctx)
	return err
}

func (a *apiHealthCheckerAdapter) GetCircuitBreakerState() string {
	// Type assert to get the concrete APIClient type
	if apiClient, ok := a.client.(*handler.APIClient); ok {
		return apiClient.GetCircuitBreakerState()
	}
	return "N/A"
}

func (a *apiHealthCheckerAdapter) GetCircuitBreakerStats() commands.CircuitBreakerStats {
	// Type assert to get the concrete APIClient type
	if apiClient, ok := a.client.(*handler.APIClient); ok {
		stats := apiClient.GetCircuitBreakerStats()
		return commands.CircuitBreakerStats{
			State:               stats.State.String(),
			ConsecutiveFailures: stats.ConsecutiveFailures,
			LastFailureTime:     stats.LastFailureTime,
			LastStateChange:     stats.LastStateChange,
		}
	}
	return commands.CircuitBreakerStats{State: "N/A"}
}

// registerCoreCommands registers all core commands with the registry
func registerCoreCommands(registry *commands.Registry, db *database.DB, userMgr *user.Manager, logger output.Logger, startTime time.Time, apiEndpoint string, apiHealthChecker commands.APIHealthChecker, ircClient commands.IRCClient) {
	// Owner verification command (Requirement 11.4, 11.6)
	_ = registry.Register(commands.NewVerifyCommand(userMgr, db))

	// Info commands
	infoCommands := commands.NewInfoCommands(apiEndpoint, apiHealthChecker)
	_ = registry.Register(commands.NewVersionCommand(infoCommands))
	_ = registry.Register(commands.NewUptimeCommand(infoCommands))
	_ = registry.Register(commands.NewStatusCommand(infoCommands))
	_ = registry.Register(commands.NewPingCommand())
	_ = registry.Register(commands.NewHelpCommand(registry))
	_ = registry.Register(commands.NewSeenCommand(db))

	// Audit command (Requirement 29.5)
	_ = registry.Register(commands.NewAuditCommand(db))

	// Metrics command (Requirement 30.4)
	_ = registry.Register(commands.NewMetricsCommand(db, startTime))

	// User management commands
	_ = registry.Register(commands.NewUserAddCommand(userMgr, db))
	_ = registry.Register(commands.NewUserRemoveCommand(userMgr, db))
	_ = registry.Register(commands.NewUserListCommand(userMgr))

	// Bot control commands
	_ = registry.Register(commands.NewPMEnableCommand(db))
	_ = registry.Register(commands.NewPMDisableCommand(db))
	_ = registry.Register(commands.NewChannelEnableCommand(db))
	_ = registry.Register(commands.NewChannelDisableCommand(db))
	_ = registry.Register(commands.NewJoinCommand(ircClient))
	_ = registry.Register(commands.NewPartCommand(ircClient))
	_ = registry.Register(commands.NewNickCommand(ircClient))
	_ = registry.Register(commands.NewQuitCommand(ircClient))

	// Channel management commands (admin/owner)
	_ = registry.Register(commands.NewOpCommand(ircClient))
	_ = registry.Register(commands.NewDeopCommand(ircClient))
	_ = registry.Register(commands.NewVoiceCommand(ircClient))
	_ = registry.Register(commands.NewDevoiceCommand(ircClient))
	_ = registry.Register(commands.NewTopicCommand(ircClient))
	_ = registry.Register(commands.NewTopicAppendCommand(ircClient))
	_ = registry.Register(commands.NewModeCommand(ircClient))
	_ = registry.Register(commands.NewInviteCommand(ircClient))

	// Moderation commands (admin/owner)
	_ = registry.Register(commands.NewKickCommand(ircClient))
	_ = registry.Register(commands.NewBanCommand(ircClient))
	_ = registry.Register(commands.NewUnbanCommand(ircClient))
	_ = registry.Register(commands.NewKickBanCommand(ircClient))
	_ = registry.Register(commands.NewMuteCommand(ircClient))
	_ = registry.Register(commands.NewUnmuteCommand(ircClient))

	logger.Info("Core commands registered")
}

// setupIRCHandlers sets up IRC event handlers
// Note: This must be called BEFORE Connect()
func setupIRCHandlers(connManager *irc.ConnectionManager, messageHandler *handler.MessageHandler, cfg *config.Config, logger output.Logger) {
	// Set up the message handler via the connection manager
	// This integrates the bot's message handler with the IRC connection lifecycle
	connManager.SetupBotMessageHandler(messageHandler, cfg, logger)
	logger.Info("IRC handlers configured")
}
