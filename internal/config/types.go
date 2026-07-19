package config

import "time"

// Config represents the complete bot configuration
type Config struct {
	Server             ServerConfig             `toml:"server"`
	Auth               AuthConfig               `toml:"auth"`
	Networks           []NetworkConfig          `toml:"networks"`
	Bot                BotConfig                `toml:"bot"`
	Limits             LimitsConfig             `toml:"limits"`
	Database           DatabaseConfig           `toml:"database"`
	Logging            LoggingConfig            `toml:"logging"`
	API                APIConfig                `toml:"api"`
	Images             ImagesConfig             `toml:"images"`
	PhoneNotifications PhoneNotificationsConfig `toml:"phone_notifications"`
	Game               GameConfig               `toml:"game"`
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

// GetMentionHistoryDepth returns the configured mention history depth, falling
// back to the documented default (20) when unset or invalid.
func (c *LimitsConfig) GetMentionHistoryDepth() int {
	if c.MentionHistoryDepth <= 0 {
		return 20
	}
	return c.MentionHistoryDepth
}

// DefaultGameDatabasePath is the Python-owned, dedicated game database.
const DefaultGameDatabasePath = "data/game.db"

// GameConfig contains only game routing, policy, and boundary settings. Secrets
// are deliberately absent: provider/API credentials remain environment-only.
type GameConfig struct {
	Enabled                       bool                `toml:"enabled"`
	Command                       string              `toml:"command"`
	PublicTitle                   string              `toml:"public_title"`
	PMEnabled                     bool                `toml:"pm_enabled"`
	PMRejectMode                  string              `toml:"pm_reject_mode"`
	ChannelPlayEnabled            bool                `toml:"channel_play_enabled"`
	ChannelHandoffNotice          bool                `toml:"channel_handoff_notice"`
	ChannelAllowlist              []GameChannel       `toml:"channel_allowlist"`
	DatabasePath                  string              `toml:"database_path"`
	DatabaseBusyTimeoutMS         int                 `toml:"database_busy_timeout_ms"`
	DatabasePoolSize              int                 `toml:"database_pool_size"`
	ActionTimeoutSeconds          int                 `toml:"action_timeout_seconds"`
	RecoveryTimeoutSeconds        int                 `toml:"recovery_timeout_seconds"`
	MenuContextTTLSeconds         int                 `toml:"menu_context_ttl_seconds"`
	MaxContinuationsPerContext    int                 `toml:"max_continuations_per_context"`
	MaxContinuationIdentities     int                 `toml:"max_continuation_identities"`
	MaxInputBytes                 int                 `toml:"max_input_bytes"`
	MaxPendingActionsPerPlayer    int                 `toml:"max_pending_actions_per_player"`
	ActionCooldownMS              int                 `toml:"action_cooldown_ms"`
	ActionBurst                   int                 `toml:"action_burst"`
	ActionWindowSeconds           int                 `toml:"action_window_seconds"`
	MaxMenuLines                  int                 `toml:"max_menu_lines"`
	MaxChoicesPerPage             int                 `toml:"max_choices_per_page"`
	PageSize                      int                 `toml:"page_size"`
	MaxNarrationBytes             int                 `toml:"max_narration_bytes"`
	StandardContentProfile        string              `toml:"standard_content_profile"`
	AdultContentEnabled           bool                `toml:"adult_content_enabled"`
	RealPersonContentEnabled      bool                `toml:"real_person_content_enabled"`
	AIEnhancementEnabled          bool                `toml:"ai_enhancement_enabled"`
	MilestoneAnnouncementsEnabled bool                `toml:"milestone_announcements_enabled"`
	SaveRetentionDays             int                 `toml:"save_retention_days"`
	SaveExpiryWarningDays         int                 `toml:"save_expiry_warning_days"`
	ActionRecordRetentionDays     int                 `toml:"action_record_retention_days"`
	ResetArchiveRetentionDays     int                 `toml:"reset_archive_retention_days"`
	RecoverySnapshotRetentionDays int                 `toml:"recovery_snapshot_retention_days"`
	AuditRetentionDays            int                 `toml:"audit_retention_days"`
	MaintenanceIntervalSeconds    int                 `toml:"maintenance_interval_seconds"`
	BackupEnabled                 bool                `toml:"backup_enabled"`
	BackupIntervalSeconds         int                 `toml:"backup_interval_seconds"`
	BackupDirectory               string              `toml:"backup_directory"`
	BackupRetentionCount          int                 `toml:"backup_retention_count"`
	ConfigRevision                int64               `toml:"config_revision"`
	ContentPolicyRevision         int64               `toml:"content_policy_revision"`
	Milestones                    GameMilestoneConfig `toml:"milestones"`
	ContentPolicy                 GameContentPolicy   `toml:"content_policy"`
	RateLimits                    GameRateLimits      `toml:"rate_limits"`
	validationErrors              []string
}

type GameChannel struct {
	Network string `toml:"network"`
	Channel string `toml:"channel"`
}

type GameMilestoneConfig struct {
	EligibleTypes []string      `toml:"eligible_types"`
	Destinations  []GameChannel `toml:"destinations"`
}

type GameContentPolicy struct {
	SexualContent     string `toml:"sexual_content"`
	DrugReferences    string `toml:"drug_references"`
	ViolenceIntensity string `toml:"violence_intensity"`
	AbusiveLanguage   string `toml:"abusive_language"`
	RealPersonContent string `toml:"real_person_content"`
}

type GameRateLimits struct {
	AI GameAIRateLimit `toml:"ai"`
}

type GameAIRateLimit struct {
	Enabled       bool `toml:"enabled"`
	Requests      int  `toml:"requests"`
	WindowSeconds int  `toml:"window_seconds"`
	Burst         int  `toml:"burst"`
}

// GameBoundaryLimits are duplicated on both sides of the HTTP boundary. The
// effective values are always the stricter (smaller) positive values.
type GameBoundaryLimits struct {
	MaxInputBytes        int
	MaxMenuLines         int
	MaxChoicesPerPage    int
	MaxNarrationBytes    int
	ActionTimeoutSeconds int
}

func (l GameBoundaryLimits) Reconcile(other GameBoundaryLimits) GameBoundaryLimits {
	return GameBoundaryLimits{
		MaxInputBytes:        minPositive(l.MaxInputBytes, other.MaxInputBytes),
		MaxMenuLines:         minPositive(l.MaxMenuLines, other.MaxMenuLines),
		MaxChoicesPerPage:    minPositive(l.MaxChoicesPerPage, other.MaxChoicesPerPage),
		MaxNarrationBytes:    minPositive(l.MaxNarrationBytes, other.MaxNarrationBytes),
		ActionTimeoutSeconds: minPositive(l.ActionTimeoutSeconds, other.ActionTimeoutSeconds),
	}
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func (c GameConfig) BoundaryLimits() GameBoundaryLimits {
	return GameBoundaryLimits{c.MaxInputBytes, c.MaxMenuLines, c.MaxChoicesPerPage, c.MaxNarrationBytes, c.ActionTimeoutSeconds}
}

// ValidationErrors returns a copy of non-fatal, game-only configuration errors.
func (c GameConfig) ValidationErrors() []string {
	return append([]string(nil), c.validationErrors...)
}
