package config

import "time"

// Config represents the complete bot configuration
type Config struct {
	Server                  ServerConfig                  `toml:"server"`
	Auth                    AuthConfig                    `toml:"auth"`
	Networks                []NetworkConfig               `toml:"networks"`
	Bot                     BotConfig                     `toml:"bot"`
	Limits                  LimitsConfig                  `toml:"limits"`
	Database                DatabaseConfig                `toml:"database"`
	Logging                 LoggingConfig                 `toml:"logging"`
	API                     APIConfig                     `toml:"api"`
	Images                  ImagesConfig                  `toml:"images"`
	PhoneNotifications      PhoneNotificationsConfig      `toml:"phone_notifications"`
	CodexResetNotifications CodexResetNotificationsConfig `toml:"codex_reset_notifications"`
}

const DefaultNetworkID = "libera"

// NetworkConfig contains one IRC network connection's settings.
type NetworkConfig struct {
	ID               string   `toml:"id"`
	Address          string   `toml:"address"`
	Port             int      `toml:"port"`
	TLS              bool     `toml:"tls"`
	Nickname         string   `toml:"nickname"`
	AltNicknames     []string `toml:"alt_nicknames"`
	Username         string   `toml:"username"`
	Realname         string   `toml:"realname"`
	MaxMessageLength int      `toml:"max_message_length"`
	SASLUsername     string   `toml:"sasl_username"`
	SASLPassword     string   `toml:"sasl_password"`
	NickServPassword string   `toml:"nickserv_password"`
	Channels         []string `toml:"channels"`
	Required         bool     `toml:"required"`
	// SASLRequired / NickServRequired let an operator explicitly opt in to
	// enforcing that the corresponding secret is present. When true and the
	// secret cannot be resolved (from TOML or the LOLO_* env vars), startup
	// fails with a clear error. They default to false so existing configs that
	// authenticate optionally (or not at all) keep starting unchanged.
	SASLRequired     bool `toml:"sasl_required"`
	NickServRequired bool `toml:"nickserv_required"`
}

// ImagesConfig contains image download settings
type ImagesConfig struct {
	DownloadChannels []string `toml:"download_channels"`
}

// PhoneNotificationsConfig contains phone notification settings
type PhoneNotificationsConfig struct {
	Active bool   `toml:"active"`
	URL    string `toml:"url"`
}

// CodexResetNotificationsConfig controls announcements for special Codex
// rate-limit reset credits. Normal quota-window resets are never announced.
type CodexResetNotificationsConfig struct {
	Enabled             bool     `toml:"enabled"`
	Network             string   `toml:"network"`
	Channels            []string `toml:"channels"`
	PollIntervalSeconds int      `toml:"poll_interval_seconds"`
	QueryTimeoutSeconds int      `toml:"query_timeout_seconds"`
	StatePath           string   `toml:"state_path"`
	CodexPath           string   `toml:"codex_path"`
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

// Server returns this network's server settings in the legacy shape used by
// the IRC runtime.
func (n NetworkConfig) Server() ServerConfig {
	return ServerConfig{
		Address:          n.Address,
		Port:             n.Port,
		TLS:              n.TLS,
		Nickname:         n.Nickname,
		AltNicknames:     n.AltNicknames,
		Username:         n.Username,
		Realname:         n.Realname,
		MaxMessageLength: n.MaxMessageLength,
	}
}

// Auth returns this network's auth settings in the legacy shape used by the
// IRC authenticator.
func (n NetworkConfig) Auth() AuthConfig {
	return AuthConfig{
		SASLUsername:     n.SASLUsername,
		SASLPassword:     n.SASLPassword,
		NickServPassword: n.NickServPassword,
	}
}

// ConfigForNetwork returns a shallow config clone with Server/Auth/Bot.Channels
// populated for the selected network. This keeps the existing IRC runtime
// components usable while the top-level config supports multiple networks.
func (c *Config) ConfigForNetwork(n NetworkConfig) *Config {
	clone := *c
	clone.Server = n.Server()
	clone.Auth = n.Auth()
	clone.Bot.Channels = append([]string(nil), n.Channels...)
	return &clone
}

// EffectiveNetworks returns the normalized network list. Load/validate fills
// this list, but this helper also preserves safe behavior for manually-created
// Config values in tests.
func (c *Config) EffectiveNetworks() []NetworkConfig {
	if len(c.Networks) > 0 {
		return c.Networks
	}
	return []NetworkConfig{legacyNetworkFromConfig(c)}
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
	RateLimitMessages       int `toml:"rate_limit_messages"`
	RateLimitWindow         int `toml:"rate_limit_window"`
	MaxMessageQueue         int `toml:"max_message_queue"`
	ReconnectDelayMin       int `toml:"reconnect_delay_min"`
	ReconnectDelayMax       int `toml:"reconnect_delay_max"`
	CommandCooldown         int `toml:"command_cooldown"`
	MentionAggregateDelayMS int `toml:"mention_aggregate_delay_ms"` // Delay to wait for overflow messages (default: 1000ms)
	MentionHistoryDepth     int `toml:"mention_history_depth"`      // Number of recent channel messages sent as mention context (default: 20)
}

// DatabaseConfig contains database settings
type DatabaseConfig struct {
	// Path is the SQLite database file path. When empty it falls back to the
	// documented default ("data/bot.db"). Kept config-driven per Requirement 6.6
	// so operators can relocate the database without editing source.
	Path                 string `toml:"path"`
	WALMode              bool   `toml:"wal_mode"`
	VacuumInterval       int    `toml:"vacuum_interval"`
	MessageRetentionDays int    `toml:"message_retention_days"`
}

// DefaultDatabasePath is the documented default location of the SQLite database
// file, used when database.path is left unset.
const DefaultDatabasePath = "data/bot.db"

// GetPath returns the configured database file path, falling back to the
// documented default ("data/bot.db") when unset.
func (c *DatabaseConfig) GetPath() string {
	if c.Path == "" {
		return DefaultDatabasePath
	}
	return c.Path
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	MaxLogSizeMB int `toml:"max_log_size_mb"`
	MaxLogFiles  int `toml:"max_log_files"`
}

// APIConfig contains Python API integration settings
type APIConfig struct {
	CircuitBreakerThreshold int        `toml:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   int        `toml:"circuit_breaker_timeout"`
	MaxRetries              int        `toml:"max_retries"`
	RetryBackoffMS          int        `toml:"retry_backoff_ms"`
	HTTP                    HTTPConfig `toml:"http"`
}

// HTTPConfig contains HTTP transport tunables for the Python API client.
// Defaults match the values previously hardcoded in the API client so that
// existing behavior is preserved when these fields are absent.
type HTTPConfig struct {
	DialTimeout           int `toml:"dial_timeout"`            // seconds; time to establish a TCP connection (default: 10)
	KeepAlive             int `toml:"keep_alive"`              // seconds; keep-alive probe interval (default: 30)
	TLSHandshakeTimeout   int `toml:"tls_handshake_timeout"`   // seconds; time allowed for the TLS handshake (default: 10)
	MaxIdleConns          int `toml:"max_idle_conns"`          // max idle connections across all hosts (default: 100)
	MaxIdleConnsPerHost   int `toml:"max_idle_conns_per_host"` // max idle connections per host (default: 10)
	IdleConnTimeout       int `toml:"idle_conn_timeout"`       // seconds; how long idle connections stay in the pool (default: 90)
	ResponseHeaderTimeout int `toml:"response_header_timeout"` // seconds; time to receive response headers after the request (default: 30)
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

// GetMentionAggregateDelayDuration returns the mention aggregate delay as a time.Duration
// Returns 1 second if not configured
func (c *LimitsConfig) GetMentionAggregateDelayDuration() time.Duration {
	if c.MentionAggregateDelayMS <= 0 {
		return 1 * time.Second // Default to 1 second
	}
	return time.Duration(c.MentionAggregateDelayMS) * time.Millisecond
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

// GetDialTimeoutDuration returns the HTTP dial timeout as a time.Duration
func (c *HTTPConfig) GetDialTimeoutDuration() time.Duration {
	return time.Duration(c.DialTimeout) * time.Second
}

// GetKeepAliveDuration returns the HTTP keep-alive interval as a time.Duration
func (c *HTTPConfig) GetKeepAliveDuration() time.Duration {
	return time.Duration(c.KeepAlive) * time.Second
}

// GetTLSHandshakeTimeoutDuration returns the TLS handshake timeout as a time.Duration
func (c *HTTPConfig) GetTLSHandshakeTimeoutDuration() time.Duration {
	return time.Duration(c.TLSHandshakeTimeout) * time.Second
}

// GetIdleConnTimeoutDuration returns the idle connection timeout as a time.Duration
func (c *HTTPConfig) GetIdleConnTimeoutDuration() time.Duration {
	return time.Duration(c.IdleConnTimeout) * time.Second
}

// GetResponseHeaderTimeoutDuration returns the response header timeout as a time.Duration
func (c *HTTPConfig) GetResponseHeaderTimeoutDuration() time.Duration {
	return time.Duration(c.ResponseHeaderTimeout) * time.Second
}

// GetPollIntervalDuration returns how often Codex reset-credit state is read.
func (c *CodexResetNotificationsConfig) GetPollIntervalDuration() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

// GetQueryTimeoutDuration returns the per-query Codex app-server timeout.
func (c *CodexResetNotificationsConfig) GetQueryTimeoutDuration() time.Duration {
	return time.Duration(c.QueryTimeoutSeconds) * time.Second
}

// GetMentionHistoryDepth returns the configured mention history depth, falling
// back to the documented default (20) when unset or invalid.
func (c *LimitsConfig) GetMentionHistoryDepth() int {
	if c.MentionHistoryDepth <= 0 {
		return 20
	}
	return c.MentionHistoryDepth
}
