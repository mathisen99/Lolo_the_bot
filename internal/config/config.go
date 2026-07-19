package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// Seed only the additive game section so omitted values receive documented
	// defaults while explicit false values in TOML remain authoritative.
	cfg.Game = DefaultGameConfig()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	normalizeNetworks(&cfg)
	applyDefaults(&cfg)
	resolveSecrets(&cfg)

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
		Networks: []NetworkConfig{
			{
				ID:               DefaultNetworkID,
				Address:          "irc.libera.chat",
				Port:             6697,
				TLS:              true,
				Nickname:         "Lolo",
				AltNicknames:     []string{"Lolo_", "Lolo__"},
				Username:         "lolo",
				Realname:         "Lolo IRC Bot",
				MaxMessageLength: 400,
				SASLUsername:     "Lolo",
				SASLPassword:     "",
				NickServPassword: "",
				Channels:         []string{"#yourchannel"},
				Required:         true,
			},
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
			RateLimitMessages:   1,
			RateLimitWindow:     1,
			MaxMessageQueue:     100,
			ReconnectDelayMin:   5,
			ReconnectDelayMax:   300,
			CommandCooldown:     3,  // 3 seconds per user per command
			MentionHistoryDepth: 20, // recent channel messages sent as mention context
		},
		Database: DatabaseConfig{
			Path:                 DefaultDatabasePath,
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
			HTTP: HTTPConfig{
				DialTimeout:           10,  // seconds
				KeepAlive:             30,  // seconds
				TLSHandshakeTimeout:   10,  // seconds
				MaxIdleConns:          100, // across all hosts
				MaxIdleConnsPerHost:   10,  // per host
				IdleConnTimeout:       90,  // seconds
				ResponseHeaderTimeout: 30,  // seconds
			},
		},
		PhoneNotifications: PhoneNotificationsConfig{
			Active: false,
			URL:    "",
		},
		Game: DefaultGameConfig(),
	}
}

func normalizeNetworks(cfg *Config) {
	if len(cfg.Networks) == 0 {
		cfg.Networks = []NetworkConfig{legacyNetworkFromConfig(cfg)}
		return
	}

	for i := range cfg.Networks {
		n := &cfg.Networks[i]
		n.ID = normalizeNetworkID(n.ID)
		if n.Port == 0 {
			n.Port = 6697
		}
		if n.MaxMessageLength == 0 {
			if cfg.Server.MaxMessageLength > 0 {
				n.MaxMessageLength = cfg.Server.MaxMessageLength
			} else {
				n.MaxMessageLength = 400
			}
		}
		if n.Nickname == "" {
			n.Nickname = cfg.Server.Nickname
		}
		if len(n.AltNicknames) == 0 {
			n.AltNicknames = cfg.Server.AltNicknames
		}
		if n.Username == "" {
			n.Username = cfg.Server.Username
		}
		if n.Realname == "" {
			n.Realname = cfg.Server.Realname
		}
		if n.SASLUsername == "" {
			n.SASLUsername = cfg.Auth.SASLUsername
		}
	}
}

// applyDefaults fills in documented defaults for optional fields that are
// absent (left as zero values) from the loaded config file. This lets existing
// config files continue to work while new operator-tunable fields fall back to
// the values that preserve prior behavior (Requirement 4.2).
func applyDefaults(cfg *Config) {
	// Conversation-history depth used for mention context.
	if cfg.Limits.MentionHistoryDepth == 0 {
		cfg.Limits.MentionHistoryDepth = 20
	}

	// Database file path falls back to the documented default so existing
	// configs without a [database].path key keep using data/bot.db.
	if cfg.Database.Path == "" {
		cfg.Database.Path = DefaultDatabasePath
	}

	// HTTP transport tunables for the Python API client. Defaults equal the
	// values that were previously hardcoded in the API client.
	http := &cfg.API.HTTP
	if http.DialTimeout == 0 {
		http.DialTimeout = 10
	}
	if http.KeepAlive == 0 {
		http.KeepAlive = 30
	}
	if http.TLSHandshakeTimeout == 0 {
		http.TLSHandshakeTimeout = 10
	}
	if http.MaxIdleConns == 0 {
		http.MaxIdleConns = 100
	}
	if http.MaxIdleConnsPerHost == 0 {
		http.MaxIdleConnsPerHost = 10
	}
	if http.IdleConnTimeout == 0 {
		http.IdleConnTimeout = 90
	}
	if http.ResponseHeaderTimeout == 0 {
		http.ResponseHeaderTimeout = 30
	}
}

// resolveSecrets fills empty per-network auth secrets from environment
// variables so that plaintext credentials stay out of committed configuration
// (Requirement 5). The .env file is loaded into the process environment before
// config.Load runs (see cmd/bot/main.go loadDotEnvFile), so os.LookupEnv sees
// those values here.
//
// Resolution order for each secret (first non-empty wins):
//  1. the value already present in the TOML config (config wins if set)
//  2. a per-network env var: LOLO_<ID>_NICKSERV_PASSWORD / LOLO_<ID>_SASL_PASSWORD
//  3. a global fallback env var: LOLO_NICKSERV_PASSWORD / LOLO_SASL_PASSWORD
//
// The network id is uppercased for the env-var name (ids are lowercase alnum
// such as "libera" or "rizon"). Secret values are never logged.
func resolveSecrets(cfg *Config) {
	for i := range cfg.Networks {
		n := &cfg.Networks[i]
		envKey := strings.ToUpper(n.ID)
		if n.NickServPassword == "" {
			n.NickServPassword = firstEnv(
				"LOLO_"+envKey+"_NICKSERV_PASSWORD",
				"LOLO_NICKSERV_PASSWORD",
			)
		}
		if n.SASLPassword == "" {
			n.SASLPassword = firstEnv(
				"LOLO_"+envKey+"_SASL_PASSWORD",
				"LOLO_SASL_PASSWORD",
			)
		}
	}
}

// firstEnv returns the value of the first environment variable that is set and
// non-empty, or "" if none of the given keys are set.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			return v
		}
	}
	return ""
}

func legacyNetworkFromConfig(cfg *Config) NetworkConfig {
	return NetworkConfig{
		ID:               DefaultNetworkID,
		Address:          cfg.Server.Address,
		Port:             cfg.Server.Port,
		TLS:              cfg.Server.TLS,
		Nickname:         cfg.Server.Nickname,
		AltNicknames:     append([]string(nil), cfg.Server.AltNicknames...),
		Username:         cfg.Server.Username,
		Realname:         cfg.Server.Realname,
		MaxMessageLength: cfg.Server.MaxMessageLength,
		SASLUsername:     cfg.Auth.SASLUsername,
		SASLPassword:     cfg.Auth.SASLPassword,
		NickServPassword: cfg.Auth.NickServPassword,
		Channels:         append([]string(nil), cfg.Bot.Channels...),
		Required:         true,
	}
}

func normalizeNetworkID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// validate checks that all required configuration fields are present and valid
func validate(cfg *Config) error {
	if err := validateNetworks(cfg.Networks); err != nil {
		return err
	}
	if err := validateSecrets(cfg.Networks); err != nil {
		return err
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
	if cfg.Limits.MentionHistoryDepth < 0 {
		return fmt.Errorf("limits.mention_history_depth must be non-negative, got %d", cfg.Limits.MentionHistoryDepth)
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

	// Validate HTTP transport tunables
	if cfg.API.HTTP.DialTimeout <= 0 {
		return fmt.Errorf("api.http.dial_timeout must be positive, got %d", cfg.API.HTTP.DialTimeout)
	}
	if cfg.API.HTTP.KeepAlive <= 0 {
		return fmt.Errorf("api.http.keep_alive must be positive, got %d", cfg.API.HTTP.KeepAlive)
	}
	if cfg.API.HTTP.TLSHandshakeTimeout <= 0 {
		return fmt.Errorf("api.http.tls_handshake_timeout must be positive, got %d", cfg.API.HTTP.TLSHandshakeTimeout)
	}
	if cfg.API.HTTP.MaxIdleConns <= 0 {
		return fmt.Errorf("api.http.max_idle_conns must be positive, got %d", cfg.API.HTTP.MaxIdleConns)
	}
	if cfg.API.HTTP.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("api.http.max_idle_conns_per_host must be positive, got %d", cfg.API.HTTP.MaxIdleConnsPerHost)
	}
	if cfg.API.HTTP.IdleConnTimeout <= 0 {
		return fmt.Errorf("api.http.idle_conn_timeout must be positive, got %d", cfg.API.HTTP.IdleConnTimeout)
	}
	if cfg.API.HTTP.ResponseHeaderTimeout <= 0 {
		return fmt.Errorf("api.http.response_header_timeout must be positive, got %d", cfg.API.HTTP.ResponseHeaderTimeout)
	}

	// Invalid game settings are intentionally non-fatal to the bot. Preserve
	// field-specific diagnostics and force the game master flag off.
	cfg.Game.validationErrors = validateGameConfig(cfg.Game, cfg.Networks)
	if len(cfg.Game.validationErrors) > 0 {
		cfg.Game.Enabled = false
	}

	return nil
}

// validateSecrets enforces that auth secrets are present when the operator has
// explicitly opted in to an auth mechanism.
//
// Behavioral-parity note (Requirement 7): IRC auth is OPTIONAL by default. The
// IRC layer (internal/irc/auth.go) attempts SASL only when both sasl_username
// and sasl_password are non-empty, and NickServ only when nickserv_password is
// non-empty; otherwise auth is silently skipped. Configs that connect without
// auth — or that set a sasl_username but no sasl_password — work today, so we
// must NOT turn them into a startup failure.
//
// Therefore a secret is treated as *required* only when the operator sets the
// explicit opt-in flag for that mechanism (sasl_required / nickserv_required).
// Default, example, and legacy configs do not set these flags, so they continue
// to start unchanged. Error messages name the expected env vars but never print
// any secret value.
func validateSecrets(networks []NetworkConfig) error {
	for i, n := range networks {
		label := fmt.Sprintf("networks[%d] (id %q)", i, n.ID)
		envKey := strings.ToUpper(n.ID)
		if n.SASLRequired && n.SASLPassword == "" {
			return fmt.Errorf("%s: SASL password is required (sasl_required = true) but none was found; set LOLO_%s_SASL_PASSWORD or LOLO_SASL_PASSWORD in your .env",
				label, envKey)
		}
		if n.NickServRequired && n.NickServPassword == "" {
			return fmt.Errorf("%s: NickServ password is required (nickserv_required = true) but none was found; set LOLO_%s_NICKSERV_PASSWORD or LOLO_NICKSERV_PASSWORD in your .env",
				label, envKey)
		}
	}
	return nil
}

func validateNetworks(networks []NetworkConfig) error {
	if len(networks) == 0 {
		return fmt.Errorf("at least one IRC network is required")
	}

	seen := make(map[string]struct{}, len(networks))
	for i, n := range networks {
		label := fmt.Sprintf("networks[%d]", i)
		if n.ID == "" {
			return fmt.Errorf("%s.id is required", label)
		}
		if _, exists := seen[n.ID]; exists {
			return fmt.Errorf("duplicate network id %q", n.ID)
		}
		seen[n.ID] = struct{}{}
		if n.Address == "" {
			return fmt.Errorf("%s.address is required", label)
		}
		if n.Port <= 0 || n.Port > 65535 {
			return fmt.Errorf("%s.port must be between 1 and 65535, got %d", label, n.Port)
		}
		if n.Nickname == "" {
			return fmt.Errorf("%s.nickname is required", label)
		}
		if n.Username == "" {
			return fmt.Errorf("%s.username is required", label)
		}
		if n.Realname == "" {
			return fmt.Errorf("%s.realname is required", label)
		}
		if n.MaxMessageLength <= 0 {
			return fmt.Errorf("%s.max_message_length must be positive, got %d", label, n.MaxMessageLength)
		}
	}
	return nil
}

// DefaultGameConfig documents the safe initial game policy. Rollout, channel
// play, milestones, adult/real-person content, and AI are all default-off.
func DefaultGameConfig() GameConfig {
	return GameConfig{
		Enabled: false, Command: "avenger", PublicTitle: "Harbor Sentinel",
		PMEnabled: true, PMRejectMode: "help", ChannelPlayEnabled: false,
		ChannelHandoffNotice: true, ChannelAllowlist: []GameChannel{},
		DatabasePath: DefaultGameDatabasePath, DatabaseBusyTimeoutMS: 5000,
		DatabasePoolSize: 4, ActionTimeoutSeconds: 10, RecoveryTimeoutSeconds: 30,
		MenuContextTTLSeconds: 900, MaxContinuationsPerContext: 12,
		MaxContinuationIdentities: 10000, MaxInputBytes: 512,
		MaxPendingActionsPerPlayer: 2, ActionCooldownMS: 750, ActionBurst: 4,
		ActionWindowSeconds: 10, MaxMenuLines: 4, MaxChoicesPerPage: 10,
		PageSize: 10, MaxNarrationBytes: 600, StandardContentProfile: "standard",
		AdultContentEnabled: false, RealPersonContentEnabled: false,
		AIEnhancementEnabled: false, MilestoneAnnouncementsEnabled: false,
		SaveRetentionDays: 180, SaveExpiryWarningDays: 30,
		ActionRecordRetentionDays: 90, ResetArchiveRetentionDays: 30,
		RecoverySnapshotRetentionDays: 30, AuditRetentionDays: 365,
		MaintenanceIntervalSeconds: 86400, BackupEnabled: true,
		BackupIntervalSeconds: 86400, BackupDirectory: "data/backups/game",
		BackupRetentionCount: 7, ConfigRevision: 1, ContentPolicyRevision: 1,
		Milestones:    GameMilestoneConfig{EligibleTypes: []string{"campaign_completed"}, Destinations: []GameChannel{}},
		ContentPolicy: GameContentPolicy{SexualContent: "exclude", DrugReferences: "non_instructional_only", ViolenceIntensity: "non_graphic", AbusiveLanguage: "exclude_targeted", RealPersonContent: "exclude"},
		RateLimits:    GameRateLimits{AI: GameAIRateLimit{Enabled: true, Requests: 2, WindowSeconds: 600, Burst: 1}},
	}
}

func validateGameConfig(c GameConfig, networks []NetworkConfig) []string {
	var problems []string
	add := func(field, reason string) { problems = append(problems, "game."+field+": "+reason) }
	if !validLowerToken(c.Command, 1, 32) {
		add("command", "must be a lowercase ASCII token of 1-32 characters")
	}
	if len(c.PublicTitle) == 0 || len([]byte(c.PublicTitle)) > 64 {
		add("public_title", "must be 1-64 bytes")
	}
	if c.PMRejectMode != "help" && c.PMRejectMode != "silent" {
		add("pm_reject_mode", "must be help or silent")
	}
	if !safeGamePath(c.DatabasePath, ".db") {
		add("database_path", "must be a normalized local path under data/ ending in .db")
	}
	if !safeGamePath(c.BackupDirectory, "") {
		add("backup_directory", "must be a normalized local path under data/")
	}
	positive := map[string]int{
		"database_busy_timeout_ms": c.DatabaseBusyTimeoutMS, "database_pool_size": c.DatabasePoolSize,
		"action_timeout_seconds": c.ActionTimeoutSeconds, "recovery_timeout_seconds": c.RecoveryTimeoutSeconds,
		"menu_context_ttl_seconds": c.MenuContextTTLSeconds, "max_continuations_per_context": c.MaxContinuationsPerContext,
		"max_continuation_identities": c.MaxContinuationIdentities, "max_input_bytes": c.MaxInputBytes,
		"max_pending_actions_per_player": c.MaxPendingActionsPerPlayer, "action_cooldown_ms": c.ActionCooldownMS,
		"action_burst": c.ActionBurst, "action_window_seconds": c.ActionWindowSeconds,
		"max_menu_lines": c.MaxMenuLines, "max_choices_per_page": c.MaxChoicesPerPage, "page_size": c.PageSize,
		"max_narration_bytes": c.MaxNarrationBytes, "save_retention_days": c.SaveRetentionDays,
		"save_expiry_warning_days": c.SaveExpiryWarningDays, "action_record_retention_days": c.ActionRecordRetentionDays,
		"reset_archive_retention_days": c.ResetArchiveRetentionDays, "recovery_snapshot_retention_days": c.RecoverySnapshotRetentionDays,
		"audit_retention_days": c.AuditRetentionDays, "maintenance_interval_seconds": c.MaintenanceIntervalSeconds,
		"backup_interval_seconds": c.BackupIntervalSeconds, "backup_retention_count": c.BackupRetentionCount,
	}
	for field, value := range positive {
		if value <= 0 {
			add(field, "must be positive")
		}
	}
	if c.MaxContinuationsPerContext > 12 {
		add("max_continuations_per_context", "must not exceed 12")
	}
	if c.MaxInputBytes > 4096 {
		add("max_input_bytes", "must not exceed 4096")
	}
	if c.PageSize > c.MaxChoicesPerPage {
		add("page_size", "must not exceed max_choices_per_page")
	}
	if c.MaxMenuLines < 2 {
		add("max_menu_lines", "must be at least 2")
	}
	if c.SaveExpiryWarningDays >= c.SaveRetentionDays {
		add("save_expiry_warning_days", "must be less than save_retention_days")
	}
	if c.AuditRetentionDays < c.ActionRecordRetentionDays {
		add("audit_retention_days", "must be at least action_record_retention_days")
	}
	if c.ConfigRevision < 1 {
		add("config_revision", "must be at least 1")
	}
	if c.ContentPolicyRevision < 1 {
		add("content_policy_revision", "must be at least 1")
	}
	if c.StandardContentProfile != "standard" {
		add("standard_content_profile", "must be standard")
	}
	allowed := map[string]map[string]bool{
		"sexual_content":      {"exclude": true},
		"drug_references":     {"exclude": true, "non_instructional_only": true},
		"violence_intensity":  {"exclude": true, "non_graphic": true},
		"abusive_language":    {"exclude": true, "exclude_targeted": true},
		"real_person_content": {"exclude": true},
	}
	values := map[string]string{"sexual_content": c.ContentPolicy.SexualContent, "drug_references": c.ContentPolicy.DrugReferences, "violence_intensity": c.ContentPolicy.ViolenceIntensity, "abusive_language": c.ContentPolicy.AbusiveLanguage, "real_person_content": c.ContentPolicy.RealPersonContent}
	for field, value := range values {
		if !allowed[field][value] {
			add("content_policy."+field, "unknown or broadening value")
		}
	}
	if c.RateLimits.AI.Requests <= 0 || c.RateLimits.AI.WindowSeconds <= 0 || c.RateLimits.AI.Burst <= 0 || c.RateLimits.AI.Burst > c.RateLimits.AI.Requests {
		add("rate_limits.ai", "requests/window/burst must be positive and burst must not exceed requests")
	}
	networkSet := map[string]bool{}
	for _, n := range networks {
		networkSet[n.ID] = true
	}
	seen := map[string]bool{}
	checkPair := func(field string, pair GameChannel) {
		key := pair.Network + "\x00" + strings.ToLower(pair.Channel)
		if !networkSet[pair.Network] {
			add(field, "references unknown network "+pair.Network)
		}
		if len(pair.Channel) < 2 || pair.Channel[0] != '#' || len([]byte(pair.Channel)) > 64 {
			add(field, "channel must begin with # and be at most 64 bytes")
		}
		if seen[key] {
			add(field, "contains duplicate network/channel pair")
		}
		seen[key] = true
	}
	for _, p := range c.ChannelAllowlist {
		checkPair("channel_allowlist", p)
	}
	for _, p := range c.Milestones.Destinations {
		checkPair("milestones.destinations", p)
	}
	for _, typ := range c.Milestones.EligibleTypes {
		if typ != "campaign_completed" {
			add("milestones.eligible_types", "unknown milestone type "+typ)
		}
	}
	return problems
}

func validLowerToken(s string, minLen, maxLen int) bool {
	if len(s) < minLen || len(s) > maxLen {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func safeGamePath(path, suffix string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "://") || strings.ContainsRune(path, '\x00') {
		return false
	}
	clean := filepath.Clean(path)
	if clean != path || clean == "data" || !strings.HasPrefix(clean, "data"+string(filepath.Separator)) {
		return false
	}
	return suffix == "" || strings.HasSuffix(clean, suffix)
}
