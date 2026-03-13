package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	defaultConfigPath = "config/bot.toml"
)

// Load reads and parses the configuration file from the specified path.
// If path is empty, it uses the default path.
func Load(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	// Check if config file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found at %s", path)
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	// Validate the configuration
	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadOrCreate attempts to load the configuration file, and if it doesn't exist,
// creates a default configuration file and returns the default config.
func LoadOrCreate(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, create default config
		fmt.Printf("Configuration file not found. Creating default configuration at %s\n", path)

		defaultCfg := DefaultConfig()
		if err := CreateDefault(path, defaultCfg); err != nil {
			return nil, fmt.Errorf("failed to create default configuration: %w", err)
		}

		return defaultCfg, nil
	}

	// File exists, try to load it
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// CreateDefault creates a default configuration file at the specified path
func CreateDefault(path string, cfg *Config) error {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create the file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close config file: %w", closeErr)
		}
	}()

	// Encode the config to TOML
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DefaultConfig returns a configuration with sensible defaults for Libera.Chat
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:          "irc.libera.chat",
			Port:             6697,
			TLS:              true,
			Nickname:         "Lolo",
			AltNicknames:     []string{"Lolo_", "Lolo__"},
			Username:         "lolo",
			Realname:         "Lolo IRC Bot",
			MaxMessageLength: 400, // Libera.Chat specific: 400 bytes for safety
		},
		Auth: AuthConfig{
			SASLUsername:     "Lolo",
			SASLPassword:     "",
			NickServPassword: "",
		},
		Bot: BotConfig{
			CommandPrefix: "!",
			Channels:      []string{"#yourchannel"},
			APIEndpoint:   "http://localhost:8000",
			APITimeout:    240,  // 240 seconds for complex multi-tool AI requests
			CallbackPort:  8001, // Port for Python API to call back
			TestMode:      false,
		},
		Limits: LimitsConfig{
			RateLimitMessages: 1,
			RateLimitWindow:   1,
			MaxMessageQueue:   100,
			ReconnectDelayMin: 5,
			ReconnectDelayMax: 300,
			CommandCooldown:   3, // 3 seconds per user per command
		},
		Database: DatabaseConfig{
			WALMode:              true,
			VacuumInterval:       86400, // 24 hours in seconds
			MessageRetentionDays: 90,
		},
		Logging: LoggingConfig{
			MaxLogSizeMB: 10,
			MaxLogFiles:  5,
		},
		API: APIConfig{
			CircuitBreakerThreshold: 5,  // failures before opening circuit
			CircuitBreakerTimeout:   30, // seconds before retry
			MaxRetries:              3,
			RetryBackoffMS:          100, // initial backoff, doubles each retry
		},
		Trivia: TriviaConfig{
			Enabled:                  true,
			DatabasePath:             "data/trivia.db",
			OpenAIModel:              "gpt-5.2",
			OpenAIReasoningEffort:    "medium",
			OpenAIAPIKeyEnv:          "OPENAI_API_KEY",
			OpenAIBaseURL:            "https://api.openai.com/v1",
			RequestTimeoutSeconds:    20,
			MaxOutputTokens:          220,
			GenerationRetryLimit:     5,
			DefaultAnswerTimeSeconds: 30,
			DefaultCodeAnswerTime:    30,
			DefaultHintsEnabled:      true,
			DefaultBasePoints:        100,
			DefaultMinimumPoints:     20,
			DefaultHintPenalty:       20,
			DefaultEnabled:           true,
			DefaultDifficulty:        "medium",
			DefaultCodeDifficulty:    "medium",
		},
		PhoneNotifications: PhoneNotificationsConfig{
			Active: false,
			URL:    "",
		},
	}
}

// validate checks that all required configuration fields are present and valid
func validate(cfg *Config) error {
	applyTriviaDefaults(cfg)

	// Validate server settings
	if cfg.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port)
	}
	if cfg.Server.Nickname == "" {
		return fmt.Errorf("server.nickname is required")
	}
	if cfg.Server.Username == "" {
		return fmt.Errorf("server.username is required")
	}
	if cfg.Server.Realname == "" {
		return fmt.Errorf("server.realname is required")
	}
	if cfg.Server.MaxMessageLength <= 0 {
		return fmt.Errorf("server.max_message_length must be positive, got %d", cfg.Server.MaxMessageLength)
	}

	// Validate bot settings
	if cfg.Bot.CommandPrefix == "" {
		return fmt.Errorf("bot.command_prefix is required")
	}
	if cfg.Bot.APIEndpoint == "" {
		return fmt.Errorf("bot.api_endpoint is required")
	}
	if cfg.Bot.APITimeout <= 0 {
		return fmt.Errorf("bot.api_timeout must be positive, got %d", cfg.Bot.APITimeout)
	}

	// Validate limits
	if cfg.Limits.RateLimitMessages <= 0 {
		return fmt.Errorf("limits.rate_limit_messages must be positive, got %d", cfg.Limits.RateLimitMessages)
	}
	if cfg.Limits.RateLimitWindow <= 0 {
		return fmt.Errorf("limits.rate_limit_window must be positive, got %d", cfg.Limits.RateLimitWindow)
	}
	if cfg.Limits.MaxMessageQueue <= 0 {
		return fmt.Errorf("limits.max_message_queue must be positive, got %d", cfg.Limits.MaxMessageQueue)
	}
	if cfg.Limits.ReconnectDelayMin <= 0 {
		return fmt.Errorf("limits.reconnect_delay_min must be positive, got %d", cfg.Limits.ReconnectDelayMin)
	}
	if cfg.Limits.ReconnectDelayMax <= 0 {
		return fmt.Errorf("limits.reconnect_delay_max must be positive, got %d", cfg.Limits.ReconnectDelayMax)
	}
	if cfg.Limits.ReconnectDelayMin > cfg.Limits.ReconnectDelayMax {
		return fmt.Errorf("limits.reconnect_delay_min (%d) cannot be greater than reconnect_delay_max (%d)",
			cfg.Limits.ReconnectDelayMin, cfg.Limits.ReconnectDelayMax)
	}
	if cfg.Limits.CommandCooldown < 0 {
		return fmt.Errorf("limits.command_cooldown must be non-negative, got %d", cfg.Limits.CommandCooldown)
	}

	// Validate database settings
	if cfg.Database.VacuumInterval <= 0 {
		return fmt.Errorf("database.vacuum_interval must be positive, got %d", cfg.Database.VacuumInterval)
	}
	if cfg.Database.MessageRetentionDays <= 0 {
		return fmt.Errorf("database.message_retention_days must be positive, got %d", cfg.Database.MessageRetentionDays)
	}

	// Validate logging settings
	if cfg.Logging.MaxLogSizeMB <= 0 {
		return fmt.Errorf("logging.max_log_size_mb must be positive, got %d", cfg.Logging.MaxLogSizeMB)
	}
	if cfg.Logging.MaxLogFiles <= 0 {
		return fmt.Errorf("logging.max_log_files must be positive, got %d", cfg.Logging.MaxLogFiles)
	}

	// Validate API settings
	if cfg.API.CircuitBreakerThreshold <= 0 {
		return fmt.Errorf("api.circuit_breaker_threshold must be positive, got %d", cfg.API.CircuitBreakerThreshold)
	}
	if cfg.API.CircuitBreakerTimeout <= 0 {
		return fmt.Errorf("api.circuit_breaker_timeout must be positive, got %d", cfg.API.CircuitBreakerTimeout)
	}
	if cfg.API.MaxRetries < 0 {
		return fmt.Errorf("api.max_retries must be non-negative, got %d", cfg.API.MaxRetries)
	}
	if cfg.API.RetryBackoffMS <= 0 {
		return fmt.Errorf("api.retry_backoff_ms must be positive, got %d", cfg.API.RetryBackoffMS)
	}

	// Validate trivia settings
	if cfg.Trivia.DatabasePath == "" {
		return fmt.Errorf("trivia.database_path is required")
	}
	if cfg.Trivia.OpenAIModel == "" {
		return fmt.Errorf("trivia.openai_model is required")
	}
	if cfg.Trivia.OpenAIReasoningEffort != "" {
		switch cfg.Trivia.OpenAIReasoningEffort {
		case "none", "low", "medium", "high", "xhigh":
		default:
			return fmt.Errorf("trivia.openai_reasoning_effort must be one of none, low, medium, high, xhigh, got %s", cfg.Trivia.OpenAIReasoningEffort)
		}
	}
	if cfg.Trivia.OpenAIBaseURL == "" {
		return fmt.Errorf("trivia.openai_base_url is required")
	}
	if cfg.Trivia.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("trivia.request_timeout_seconds must be positive, got %d", cfg.Trivia.RequestTimeoutSeconds)
	}
	if cfg.Trivia.MaxOutputTokens <= 0 {
		return fmt.Errorf("trivia.max_output_tokens must be positive, got %d", cfg.Trivia.MaxOutputTokens)
	}
	if cfg.Trivia.GenerationRetryLimit <= 0 {
		return fmt.Errorf("trivia.generation_retry_limit must be positive, got %d", cfg.Trivia.GenerationRetryLimit)
	}
	if cfg.Trivia.DefaultAnswerTimeSeconds <= 0 {
		return fmt.Errorf("trivia.default_answer_time_seconds must be positive, got %d", cfg.Trivia.DefaultAnswerTimeSeconds)
	}
	if cfg.Trivia.DefaultCodeAnswerTime <= 0 {
		return fmt.Errorf("trivia.default_code_answer_time_seconds must be positive, got %d", cfg.Trivia.DefaultCodeAnswerTime)
	}
	if cfg.Trivia.DefaultBasePoints <= 0 {
		return fmt.Errorf("trivia.default_base_points must be positive, got %d", cfg.Trivia.DefaultBasePoints)
	}
	if cfg.Trivia.DefaultMinimumPoints < 0 {
		return fmt.Errorf("trivia.default_minimum_points must be non-negative, got %d", cfg.Trivia.DefaultMinimumPoints)
	}
	if cfg.Trivia.DefaultMinimumPoints > cfg.Trivia.DefaultBasePoints {
		return fmt.Errorf("trivia.default_minimum_points (%d) cannot exceed default_base_points (%d)",
			cfg.Trivia.DefaultMinimumPoints, cfg.Trivia.DefaultBasePoints)
	}
	if cfg.Trivia.DefaultHintPenalty < 0 {
		return fmt.Errorf("trivia.default_hint_penalty must be non-negative, got %d", cfg.Trivia.DefaultHintPenalty)
	}
	if cfg.Trivia.DefaultDifficulty != "" {
		switch cfg.Trivia.DefaultDifficulty {
		case "easy", "medium", "hard":
		default:
			return fmt.Errorf("trivia.default_difficulty must be one of easy, medium, hard, got %s", cfg.Trivia.DefaultDifficulty)
		}
	}
	if cfg.Trivia.DefaultCodeDifficulty != "" {
		switch cfg.Trivia.DefaultCodeDifficulty {
		case "easy", "medium", "hard":
		default:
			return fmt.Errorf("trivia.default_code_difficulty must be one of easy, medium, hard, got %s", cfg.Trivia.DefaultCodeDifficulty)
		}
	}

	return nil
}

func applyTriviaDefaults(cfg *Config) {
	triviaSectionMissing := cfg.Trivia.DatabasePath == "" &&
		cfg.Trivia.OpenAIModel == "" &&
		cfg.Trivia.OpenAIAPIKeyEnv == "" &&
		cfg.Trivia.OpenAIBaseURL == "" &&
		cfg.Trivia.RequestTimeoutSeconds == 0 &&
		cfg.Trivia.MaxOutputTokens == 0 &&
		cfg.Trivia.GenerationRetryLimit == 0 &&
		cfg.Trivia.DefaultAnswerTimeSeconds == 0 &&
		cfg.Trivia.DefaultCodeAnswerTime == 0 &&
		cfg.Trivia.DefaultBasePoints == 0 &&
		cfg.Trivia.DefaultMinimumPoints == 0 &&
		cfg.Trivia.DefaultHintPenalty == 0 &&
		cfg.Trivia.DefaultDifficulty == "" &&
		cfg.Trivia.DefaultCodeDifficulty == "" &&
		!cfg.Trivia.DefaultHintsEnabled &&
		!cfg.Trivia.DefaultEnabled &&
		!cfg.Trivia.Enabled

	if triviaSectionMissing {
		cfg.Trivia.Enabled = true
		cfg.Trivia.DefaultHintsEnabled = true
		cfg.Trivia.DefaultEnabled = true
		cfg.Trivia.DefaultAnswerTimeSeconds = 30
		cfg.Trivia.DefaultCodeAnswerTime = 30
		cfg.Trivia.DefaultBasePoints = 100
		cfg.Trivia.DefaultMinimumPoints = 20
		cfg.Trivia.DefaultHintPenalty = 20
		cfg.Trivia.DefaultDifficulty = "medium"
		cfg.Trivia.DefaultCodeDifficulty = "medium"
	}

	if cfg.Trivia.DatabasePath == "" {
		cfg.Trivia.DatabasePath = "data/trivia.db"
	}
	if cfg.Trivia.OpenAIModel == "" {
		cfg.Trivia.OpenAIModel = "gpt-5.2"
	}
	if cfg.Trivia.OpenAIReasoningEffort == "" {
		cfg.Trivia.OpenAIReasoningEffort = "medium"
	}
	if cfg.Trivia.OpenAIAPIKeyEnv == "" {
		cfg.Trivia.OpenAIAPIKeyEnv = "OPENAI_API_KEY"
	}
	if cfg.Trivia.OpenAIBaseURL == "" {
		cfg.Trivia.OpenAIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.Trivia.RequestTimeoutSeconds <= 0 {
		cfg.Trivia.RequestTimeoutSeconds = 20
	}
	if cfg.Trivia.MaxOutputTokens <= 0 {
		cfg.Trivia.MaxOutputTokens = 220
	}
	if cfg.Trivia.GenerationRetryLimit <= 0 {
		cfg.Trivia.GenerationRetryLimit = 5
	}
	if cfg.Trivia.DefaultAnswerTimeSeconds <= 0 {
		cfg.Trivia.DefaultAnswerTimeSeconds = 30
	}
	if cfg.Trivia.DefaultCodeAnswerTime <= 0 {
		cfg.Trivia.DefaultCodeAnswerTime = cfg.Trivia.DefaultAnswerTimeSeconds
	}
	if cfg.Trivia.DefaultBasePoints <= 0 {
		cfg.Trivia.DefaultBasePoints = 100
	}
	if cfg.Trivia.DefaultMinimumPoints < 0 {
		cfg.Trivia.DefaultMinimumPoints = 20
	}
	if cfg.Trivia.DefaultHintPenalty < 0 {
		cfg.Trivia.DefaultHintPenalty = 20
	}
	if cfg.Trivia.DefaultDifficulty == "" {
		cfg.Trivia.DefaultDifficulty = "medium"
	}
	if cfg.Trivia.DefaultCodeDifficulty == "" {
		cfg.Trivia.DefaultCodeDifficulty = cfg.Trivia.DefaultDifficulty
	}
}
