package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
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
	"github.com/yourusername/lolo/internal/reminder"
	"github.com/yourusername/lolo/internal/shutdown"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/trivia"
	"github.com/yourusername/lolo/internal/user"
)

type networkRuntime struct {
	id            string
	required      bool
	cfg           *config.Config
	connManager   *irc.ConnectionManager
	messageHandle *handler.MessageHandler
	ownerVerifier *irc.OwnerVerifier
}

func main() {
	// Parse command-line flags
	rollbackFlag := flag.Bool("rollback", false, "Rollback the last applied database migration")
	flag.Parse()

	// Create logger first for colored output
	logger := output.NewColorLogger()
	logger.Info("Lolo IRC Bot - Starting...")

	// Load .env file for Go-side environment variables (e.g., OPENAI_API_KEY for trivia).
	// Missing .env is not fatal; existing process environment always takes precedence.
	loadDotEnvFile(".env", logger)

	// Load or create configuration first to check test mode
	cfg, err := config.LoadOrCreate("config/bot.toml")
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}
	logger.Success("Configuration loaded")
	networkConfigs := cfg.EffectiveNetworks()

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

	// Initialize dedicated trivia database and manager
	triviaDBPath := cfg.Trivia.DatabasePath
	if cfg.Bot.TestMode {
		triviaDBPath = ":memory:"
	}

	triviaDefaults := trivia.StoreDefaults{
		Settings: trivia.ChannelSettings{
			AnswerTimeSeconds:     cfg.Trivia.DefaultAnswerTimeSeconds,
			CodeAnswerTimeSeconds: cfg.Trivia.DefaultCodeAnswerTime,
			TriviaHintsEnabled:    cfg.Trivia.DefaultHintsEnabled,
			CodeHintsEnabled:      cfg.Trivia.DefaultHintsEnabled,
			AntiCheatEnabled:      true,
			BasePoints:            cfg.Trivia.DefaultBasePoints,
			MinimumPoints:         cfg.Trivia.DefaultMinimumPoints,
			HintPenalty:           cfg.Trivia.DefaultHintPenalty,
			Enabled:               cfg.Trivia.DefaultEnabled,
			Difficulty:            cfg.Trivia.DefaultDifficulty,
			CodeDifficulty:        cfg.Trivia.DefaultCodeDifficulty,
		},
	}

	triviaStore, err := trivia.NewStore(triviaDBPath, triviaDefaults)
	if err != nil {
		logger.Error("Failed to initialize trivia database: %v", err)
		_ = db.Close()
		os.Exit(1)
	}
	logger.Success("Trivia database initialized")

	triviaGenerator := trivia.NewGenerator(trivia.GeneratorConfig{
		Enabled:         cfg.Trivia.Enabled,
		APIKeyEnv:       cfg.Trivia.OpenAIAPIKeyEnv,
		BaseURL:         cfg.Trivia.OpenAIBaseURL,
		Model:           cfg.Trivia.OpenAIModel,
		ReasoningEffort: cfg.Trivia.OpenAIReasoningEffort,
		RequestTimeout:  cfg.Trivia.GetRequestTimeoutDuration(),
		MaxOutputTokens: cfg.Trivia.MaxOutputTokens,
	}, logger)

	// Handle rollback if requested
	if *rollbackFlag {
		logger.Info("Rolling back last migration...")
		if err := db.Rollback(); err != nil {
			logger.Error("Rollback failed: %v", err)
			_ = triviaStore.Close()
			_ = db.Close()
			os.Exit(1)
		}
		logger.Success("Migration rolled back successfully")
		_ = triviaStore.Close()
		_ = db.Close()
		os.Exit(0)
	}

	// Initialize output with error logging
	out, err := output.NewOutput("data/error.log")
	if err != nil {
		logger.Error("Failed to initialize output: %v", err)
		_ = triviaStore.Close()
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
		_ = triviaStore.Close()
		_ = db.Close()
		os.Exit(1)
	}
	logger.Success("Database maintenance scheduler started")

	// Create user manager
	userMgr := user.NewManager(db)

	// Check if owner password needs to be set (Requirement 11.2)
	hasPassword, err := userMgr.HasOwnerPassword()
	if err != nil {
		logger.Error("Failed to check owner password: %v", err)
		_ = triviaStore.Close()
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
			_ = triviaStore.Close()
			_ = db.Close()
			os.Exit(1)
		}

		// Set the owner password (hashed with bcrypt)
		if err := userMgr.SetOwnerPassword(password); err != nil {
			logger.Error("Failed to set owner password: %v", err)
			_ = triviaStore.Close()
			_ = db.Close()
			os.Exit(1)
		}

		logger.Success("Owner password set successfully!")
		logger.Info("To become owner, send this command via PM (NOT in channel):")
		ownerVerifyNick := "Lolo"
		if len(networkConfigs) > 0 && networkConfigs[0].Nickname != "" {
			ownerVerifyNick = networkConfigs[0].Nickname
		}
		logger.Info("  /msg %s !verify %s", ownerVerifyNick, password)
	}

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

	// Create error handler
	errorHandler := errors.NewErrorHandler(out)

	runtimes := make([]*networkRuntime, 0, len(networkConfigs))
	for _, netCfg := range networkConfigs {
		networkID := netCfg.ID
		networkCfg := cfg.ConfigForNetwork(netCfg)

		connManager := irc.NewConnectionManagerForNetwork(networkID, networkCfg, logger, db, userMgr)
		registry := commands.NewRegistry()

		var ownerVerifier *irc.OwnerVerifier
		var ownerVerifierFunc func(string) (bool, error)
		if networkID == "rizon" {
			ownerVerifier = irc.NewOwnerVerifier(connManager.GetClient())
			ownerVerifierFunc = ownerVerifier.Verify
		}

		dispatcher := commands.NewDispatcherForNetwork(registry, userMgr, cfg.Bot.CommandPrefix, networkID, ownerVerifierFunc)
		channelPrefixes, err := db.ListChannelCommandPrefixesForNetwork(networkID)
		if err != nil {
			logger.Error("Failed to load channel command prefixes for %s: %v", networkID, err)
			_ = triviaStore.Close()
			_ = db.Close()
			os.Exit(1)
		}
		for channel, prefix := range channelPrefixes {
			dispatcher.SetChannelPrefix(channel, prefix)
		}

		triviaManager := trivia.NewManager(trivia.ManagerConfig{
			Network:           networkID,
			Store:             triviaStore,
			Generator:         triviaGenerator,
			Logger:            logger,
			GenerationRetries: cfg.Trivia.GenerationRetryLimit,
		})

		registerCoreCommands(registry, dispatcher, db, userMgr, logger, startTime, cfg.Bot.APIEndpoint, apiHealthChecker, connManager.GetClient(), triviaManager)

		msgSplitter := splitter.New(netCfg.MaxMessageLength)
		messageHandler := handler.NewMessageHandler(&handler.MessageHandlerConfig{
			Network:                  networkID,
			Dispatcher:               dispatcher,
			APIClient:                apiClient,
			UserManager:              userMgr,
			DB:                       db,
			Logger:                   logger,
			ErrorHandler:             errorHandler,
			Splitter:                 msgSplitter,
			BotNick:                  netCfg.Nickname,
			TestMode:                 cfg.Bot.TestMode,
			ImageDownloadChannels:    cfg.Images.DownloadChannels,
			PhoneNotificationsActive: cfg.PhoneNotifications.Active,
			PhoneNotificationsURL:    cfg.PhoneNotifications.URL,
			MentionAggregateDelay:    cfg.Limits.GetMentionAggregateDelayDuration(),
			TriviaManager:            triviaManager,
		})

		channelTracker := irc.NewChannelTrackerForNetwork(db, logger, networkID, netCfg.Nickname)
		connManager.SetChannelUserTracker(channelTracker)

		if !cfg.Bot.TestMode {
			reminderChecker := reminder.NewCheckerForNetwork(cfg.Bot.APIEndpoint, connManager.GetClient(), logger, networkID)
			connManager.SetJoinHandler(reminderChecker.OnJoinAsync)
			logger.Success("Reminder checker initialized for %s (on-join delivery enabled)", networkID)
		}

		setupIRCHandlers(connManager, messageHandler, networkCfg, logger)

		runtimes = append(runtimes, &networkRuntime{
			id:            networkID,
			required:      netCfg.Required,
			cfg:           networkCfg,
			connManager:   connManager,
			messageHandle: messageHandler,
			ownerVerifier: ownerVerifier,
		})
	}

	// Start callback server for Python API to call back (IRC command tool)
	var callbackServer *callback.Server
	if cfg.Bot.CallbackPort > 0 {
		callbackServer = callback.NewServer(nil, logger, cfg.Bot.CallbackPort)
		callbackServer.SetDatabase(db) // Enable bot_status and channel_info commands
		for _, rt := range runtimes {
			callbackServer.RegisterNetwork(rt.id, rt.connManager.GetClient())
		}
		if err := callbackServer.Start(); err != nil {
			logger.Error("Failed to start callback server: %v", err)
		} else {
			logger.Success("Callback server started on port %d", cfg.Bot.CallbackPort)
		}
	}

	for _, rt := range runtimes {
		runtime := rt
		runtime.connManager.SetNoticeHandler(func(source, message string) {
			if runtime.ownerVerifier != nil {
				runtime.ownerVerifier.HandleNotice(source, message)
			}
			if callbackServer != nil {
				callbackServer.HandleServiceResponseForNetwork(runtime.id, source, message)
			}
		})
		if callbackServer != nil {
			runtime.connManager.SetNumericHandler(func(numeric int, params []string) {
				callbackServer.HandleNumericResponseForNetwork(runtime.id, numeric, params)
			})
			runtime.connManager.SetCTCPResponseHandler(func(source, ctcpType, response string) {
				callbackServer.HandleCTCPResponseForNetwork(runtime.id, source, ctcpType, response)
			})
		}
	}

	// Cache command metadata from Python API (Requirement 31.5)
	if healthResp != nil {
		logger.Info("Fetching command metadata from Python API...")
		metadataCtx, metadataCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer metadataCancel()

		for _, rt := range runtimes {
			if err := rt.messageHandle.CacheCommandMetadata(metadataCtx); err != nil {
				logger.Warning("Failed to cache command metadata for %s: %v", rt.id, err)
				logger.Warning("Help text and validation may not be available for API commands")
			}
		}
	}

	// Set up shutdown handler with 5-second forced timeout (Requirement 10.5)
	shutdownHandler := shutdown.NewHandler(logger, 5*time.Second)

	// Register shutdown functions in order
	// 1. Send IRC QUIT message (Requirement 10.1)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Sending QUIT message to IRC servers...")
		var firstErr error
		for _, rt := range runtimes {
			if err := rt.connManager.Disconnect(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s disconnect failed: %w", rt.id, err)
			}
		}
		return firstErr
	})

	// 2. Stop reconnection manager
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Stopping reconnection managers...")
		for _, rt := range runtimes {
			rt.connManager.StopReconnectionManager()
		}
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

	// 3.6. Shutdown message handler (cancels pending mention aggregations)
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Shutting down message handlers...")
		for _, rt := range runtimes {
			rt.messageHandle.Shutdown()
		}
		return nil
	})

	// 3.7. Close trivia database
	shutdownHandler.RegisterShutdownFunc(func() error {
		logger.Info("Closing trivia database...")
		if err := triviaStore.Close(); err != nil {
			return fmt.Errorf("failed to close trivia database: %w", err)
		}
		logger.Success("Trivia database connection closed")
		return nil
	})

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

	for _, rt := range runtimes {
		startNetworkRunLoop(rt, shutdownHandler, logger)
	}

	time.Sleep(100 * time.Millisecond)

	for _, rt := range runtimes {
		if err := connectNetworkRuntime(rt, logger); err != nil {
			if rt.required {
				logger.Error("Failed to connect required IRC network %s: %v", rt.id, err)
				shutdownHandler.Shutdown()
				<-shutdownHandler.Done()
				os.Exit(1)
			}
			logger.Warning("Failed to connect optional IRC network %s: %v", rt.id, err)
			go retryOptionalNetworkRuntime(rt, shutdownHandler, logger, cfg.Limits.GetReconnectDelayMinDuration())
			continue
		}
	}

	logger.Success("Bot initialization complete. IRC runtimes are ready.")

	// Wait for graceful shutdown to complete
	<-shutdownHandler.Done()
}

func startNetworkRunLoop(rt *networkRuntime, shutdownHandler *shutdown.Handler, logger output.Logger) {
	go func() {
		for {
			if err := rt.connManager.Run(); err != nil {
				logger.Error("[%s] IRC client error: %v", rt.id, err)

				select {
				case <-shutdownHandler.Done():
					return
				default:
					logger.Info("[%s] Connection lost, reconnection manager will handle it...", rt.id)
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}
			break
		}
	}()
}

func connectNetworkRuntime(rt *networkRuntime, logger output.Logger) error {
	logger.Info("[%s] Connecting to IRC...", rt.id)
	if err := rt.connManager.Connect(); err != nil {
		return err
	}

	rt.connManager.StartReconnectionManager()
	if err := rt.connManager.JoinChannels(); err != nil {
		logger.Error("[%s] Failed to join channels: %v", rt.id, err)
	}
	rt.connManager.StartPingMonitor()
	logger.Success("[%s] Connected and ready", rt.id)
	return nil
}

func retryOptionalNetworkRuntime(rt *networkRuntime, shutdownHandler *shutdown.Handler, logger output.Logger, delay time.Duration) {
	if delay <= 0 {
		delay = 30 * time.Second
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-shutdownHandler.Done():
			return
		case <-timer.C:
			if err := connectNetworkRuntime(rt, logger); err != nil {
				logger.Warning("[%s] Optional network reconnect failed: %v", rt.id, err)
				timer.Reset(delay)
				continue
			}
			return
		}
	}
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
func registerCoreCommands(registry *commands.Registry, dispatcher *commands.Dispatcher, db *database.DB, userMgr *user.Manager, logger output.Logger, startTime time.Time, apiEndpoint string, apiHealthChecker commands.APIHealthChecker, ircClient commands.IRCClient, triviaManager *trivia.Manager) {
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
	_ = registry.Register(commands.NewPrefixCommand(db, dispatcher))
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

	// Trivia commands
	_ = registry.Register(commands.NewTriviaCommand(triviaManager))
	_ = registry.Register(commands.NewQuizCommand(triviaManager))
	_ = registry.Register(commands.NewCodeCommand(triviaManager))
	_ = registry.Register(commands.NewHintCommand(triviaManager))
	_ = registry.Register(commands.NewTriviaRulesCommand(triviaManager))
	_ = registry.Register(commands.NewQuizRulesCommand(triviaManager))
	_ = registry.Register(commands.NewTop10Command(triviaManager))
	_ = registry.Register(commands.NewScoreCommand(triviaManager, db))
	_ = registry.Register(commands.NewTriviaSettingsCommand(triviaManager, db))

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

// loadDotEnvFile loads KEY=VALUE pairs from a .env file into the process environment.
// Existing environment variables are preserved.
func loadDotEnvFile(path string, logger output.Logger) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("No %s file found, using existing environment", path)
			return
		}
		logger.Warning("Failed to open %s: %v", path, err)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warning("Failed to close %s: %v", path, closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	loadedCount := 0
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		equalIndex := strings.IndexByte(line, '=')
		if equalIndex <= 0 {
			logger.Warning("Skipping invalid %s line %d", path, lineNumber)
			continue
		}

		key := strings.TrimSpace(line[:equalIndex])
		value := strings.TrimSpace(line[equalIndex+1:])
		if key == "" {
			logger.Warning("Skipping invalid %s line %d", path, lineNumber)
			continue
		}

		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			logger.Warning("Failed setting env var %s from %s: %v", key, path, err)
			continue
		}
		loadedCount++
	}

	if err := scanner.Err(); err != nil {
		logger.Warning("Failed reading %s: %v", path, err)
		return
	}

	logger.Info("Loaded %d environment variables from %s", loadedCount, path)
}
