package config

import "time"

// Config represents the complete bot configuration
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Auth     AuthConfig     `toml:"auth"`
	Bot      BotConfig      `toml:"bot"`
	Limits   LimitsConfig   `toml:"limits"`
	Database DatabaseConfig `toml:"database"`
	Logging  LoggingConfig  `toml:"logging"`
	API      APIConfig      `toml:"api"`
	Images   ImagesConfig   `toml:"images"`
}

// ImagesConfig contains image download settings
type ImagesConfig struct {
	DownloadChannels []string `toml:"download_channels"`
}

// ServerConfig contains IRC server connection settings
type ServerConfig struct {
	Address          string   `toml:"address"`
	Port             int      `toml:"port"`
	TLS              bool     `toml:"tls"`
	Nickname         string   `toml:"nickname"`
	AltNicknames     []string `toml:"alt_nicknames"`
	Username         string   `toml:"username"`
	Realname         string   `toml:"realname"`
	MaxMessageLength int      `toml:"max_message_length"`
}

// AuthConfig contains authentication credentials
type AuthConfig struct {
	SASLUsername     string `toml:"sasl_username"`
	SASLPassword     string `toml:"sasl_password"`
	NickServPassword string `toml:"nickserv_password"`
}

// BotConfig contains bot behavior settings
type BotConfig struct {
	CommandPrefix string   `toml:"command_prefix"`
	Channels      []string `toml:"channels"`
	APIEndpoint   string   `toml:"api_endpoint"`
	APITimeout    int      `toml:"api_timeout"`
	CallbackPort  int      `toml:"callback_port"`
	TestMode      bool     `toml:"test_mode"`
}

// LimitsConfig contains rate limiting and backoff settings
type LimitsConfig struct {
	RateLimitMessages int `toml:"rate_limit_messages"`
	RateLimitWindow   int `toml:"rate_limit_window"`
	MaxMessageQueue   int `toml:"max_message_queue"`
	ReconnectDelayMin int `toml:"reconnect_delay_min"`
	ReconnectDelayMax int `toml:"reconnect_delay_max"`
	CommandCooldown   int `toml:"command_cooldown"`
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	WALMode              bool `toml:"wal_mode"`
	VacuumInterval       int  `toml:"vacuum_interval"`
	MessageRetentionDays int  `toml:"message_retention_days"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	MaxLogSizeMB int `toml:"max_log_size_mb"`
	MaxLogFiles  int `toml:"max_log_files"`
}

// APIConfig contains Python API integration settings
type APIConfig struct {
	CircuitBreakerThreshold int `toml:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   int `toml:"circuit_breaker_timeout"`
	MaxRetries              int `toml:"max_retries"`
	RetryBackoffMS          int `toml:"retry_backoff_ms"`
}

// GetAPITimeoutDuration returns the API timeout as a time.Duration
func (c *BotConfig) GetAPITimeoutDuration() time.Duration {
	return time.Duration(c.APITimeout) * time.Second
}

// GetReconnectDelayMinDuration returns the minimum reconnect delay as a time.Duration
func (c *LimitsConfig) GetReconnectDelayMinDuration() time.Duration {
	return time.Duration(c.ReconnectDelayMin) * time.Second
}

// GetReconnectDelayMaxDuration returns the maximum reconnect delay as a time.Duration
func (c *LimitsConfig) GetReconnectDelayMaxDuration() time.Duration {
	return time.Duration(c.ReconnectDelayMax) * time.Second
}

// GetCommandCooldownDuration returns the command cooldown as a time.Duration
func (c *LimitsConfig) GetCommandCooldownDuration() time.Duration {
	return time.Duration(c.CommandCooldown) * time.Second
}

// GetVacuumIntervalDuration returns the vacuum interval as a time.Duration
func (c *DatabaseConfig) GetVacuumIntervalDuration() time.Duration {
	return time.Duration(c.VacuumInterval) * time.Second
}

// GetCircuitBreakerTimeoutDuration returns the circuit breaker timeout as a time.Duration
func (c *APIConfig) GetCircuitBreakerTimeoutDuration() time.Duration {
	return time.Duration(c.CircuitBreakerTimeout) * time.Second
}

// GetRetryBackoffDuration returns the initial retry backoff as a time.Duration
func (c *APIConfig) GetRetryBackoffDuration() time.Duration {
	return time.Duration(c.RetryBackoffMS) * time.Millisecond
}
