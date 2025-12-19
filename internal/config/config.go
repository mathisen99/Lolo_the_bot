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
			APITimeout:    240, // 240 seconds for complex multi-tool AI requests
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
	}
}

// validate checks that all required configuration fields are present and valid
func validate(cfg *Config) error {
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

	return nil
}
