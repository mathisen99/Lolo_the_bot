package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLegacyConfigCreatesLiberaNetwork(t *testing.T) {
	path := writeConfig(t, `
[server]
address = "irc.libera.chat"
port = 6697
tls = true
nickname = "Lolo"
username = "lolo"
realname = "Lolo IRC Bot"
max_message_length = 400

[auth]
sasl_username = "Lolo"
sasl_password = ""
nickserv_password = ""

[bot]
command_prefix = "!"
channels = ["#mathizen"]
api_endpoint = "http://localhost:8000"
api_timeout = 240

[limits]
rate_limit_messages = 1
rate_limit_window = 1
max_message_queue = 100
reconnect_delay_min = 5
reconnect_delay_max = 300
command_cooldown = 3

[database]
wal_mode = true
vacuum_interval = 86400
message_retention_days = 90

[logging]
max_log_size_mb = 10
max_log_files = 5

[api]
circuit_breaker_threshold = 5
circuit_breaker_timeout = 30
max_retries = 3
retry_backoff_ms = 100
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load legacy config failed: %v", err)
	}

	if len(cfg.Networks) != 1 {
		t.Fatalf("expected one legacy network, got %d", len(cfg.Networks))
	}
	if got := cfg.Networks[0].ID; got != DefaultNetworkID {
		t.Fatalf("expected legacy network id %q, got %q", DefaultNetworkID, got)
	}
	if got := cfg.Networks[0].Channels; len(got) != 1 || got[0] != "#mathizen" {
		t.Fatalf("expected legacy channels copied, got %#v", got)
	}
	if !cfg.Networks[0].Required {
		t.Fatalf("expected legacy network to be required")
	}
}

func TestLoadMultiNetworkRejectsDuplicateIDs(t *testing.T) {
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")+"\n"+networkBlock("LIBERA", "irc.rizon.net")))

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected duplicate network id error")
	}
	if !strings.Contains(err.Error(), "duplicate network id") {
		t.Fatalf("expected duplicate network id error, got %v", err)
	}
}

func TestLoadMultiNetworkConfig(t *testing.T) {
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")+"\n"+networkBlock("rizon", "irc.rizon.net")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load multi-network config failed: %v", err)
	}

	if len(cfg.Networks) != 2 {
		t.Fatalf("expected two networks, got %d", len(cfg.Networks))
	}
	if cfg.Networks[1].ID != "rizon" || cfg.Networks[1].Channels[0] != "#mathizen" {
		t.Fatalf("expected rizon #mathizen network, got %#v", cfg.Networks[1])
	}
}

func TestLoadAppliesHTTPAndHistoryDefaults(t *testing.T) {
	// A config that omits [api.http] and limits.mention_history_depth should
	// still load and fall back to the documented defaults (Requirement 4.2).
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Limits.MentionHistoryDepth; got != 20 {
		t.Fatalf("expected default mention_history_depth 20, got %d", got)
	}

	want := HTTPConfig{
		DialTimeout:           10,
		KeepAlive:             30,
		TLSHandshakeTimeout:   10,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90,
		ResponseHeaderTimeout: 30,
	}
	if cfg.API.HTTP != want {
		t.Fatalf("expected default HTTP config %#v, got %#v", want, cfg.API.HTTP)
	}
}

func TestLoadAppliesDatabasePathDefault(t *testing.T) {
	// A config that omits [database].path should fall back to the documented
	// default so existing configs keep using data/bot.db (Requirement 6.6).
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Database.Path; got != DefaultDatabasePath {
		t.Fatalf("expected default database path %q, got %q", DefaultDatabasePath, got)
	}
	if got := cfg.Database.GetPath(); got != DefaultDatabasePath {
		t.Fatalf("expected GetPath default %q, got %q", DefaultDatabasePath, got)
	}
}

func TestLoadRespectsExplicitDatabasePath(t *testing.T) {
	path := writeConfig(t, networkBlock("libera", "irc.libera.chat")+`
[bot]
command_prefix = "!"
api_endpoint = "http://localhost:8000"
api_timeout = 240

[limits]
rate_limit_messages = 1
rate_limit_window = 1
max_message_queue = 100
reconnect_delay_min = 5
reconnect_delay_max = 300
command_cooldown = 3

[database]
path = "/var/lib/lolo/custom.db"
wal_mode = true
vacuum_interval = 86400
message_retention_days = 90

[logging]
max_log_size_mb = 10
max_log_files = 5

[api]
circuit_breaker_threshold = 5
circuit_breaker_timeout = 30
max_retries = 3
retry_backoff_ms = 100
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Database.GetPath(); got != "/var/lib/lolo/custom.db" {
		t.Fatalf("expected explicit database path, got %q", got)
	}
}

func TestLoadRespectsExplicitHTTPAndHistoryValues(t *testing.T) {
	path := writeConfig(t, tunableConfig("mention_history_depth = 5", `
[api.http]
dial_timeout = 15
keep_alive = 45
tls_handshake_timeout = 12
max_idle_conns = 200
max_idle_conns_per_host = 20
idle_conn_timeout = 120
response_header_timeout = 60
`))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Limits.MentionHistoryDepth; got != 5 {
		t.Fatalf("expected mention_history_depth 5, got %d", got)
	}
	want := HTTPConfig{
		DialTimeout:           15,
		KeepAlive:             45,
		TLSHandshakeTimeout:   12,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       120,
		ResponseHeaderTimeout: 60,
	}
	if cfg.API.HTTP != want {
		t.Fatalf("expected HTTP config %#v, got %#v", want, cfg.API.HTTP)
	}
}

func TestLoadRejectsInvalidHTTPValue(t *testing.T) {
	path := writeConfig(t, tunableConfig("", `
[api.http]
dial_timeout = -1
`))

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for negative dial_timeout")
	}
	if !strings.Contains(err.Error(), "api.http.dial_timeout") {
		t.Fatalf("expected dial_timeout validation error, got %v", err)
	}
}

func TestLoadRejectsNegativeHistoryDepth(t *testing.T) {
	path := writeConfig(t, tunableConfig("mention_history_depth = -5", ""))

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for negative mention_history_depth")
	}
	if !strings.Contains(err.Error(), "mention_history_depth") {
		t.Fatalf("expected mention_history_depth validation error, got %v", err)
	}
}

// tunableConfig builds a complete single-network config, optionally injecting an
// extra line into the [limits] table and appending an extra table block (such
// as [api.http]). This avoids duplicating the [limits]/[api] tables.
func tunableConfig(extraLimit, extraBlock string) string {
	return networkBlock("libera", "irc.libera.chat") + `
[bot]
command_prefix = "!"
api_endpoint = "http://localhost:8000"
api_timeout = 240

[limits]
rate_limit_messages = 1
rate_limit_window = 1
max_message_queue = 100
reconnect_delay_min = 5
reconnect_delay_max = 300
command_cooldown = 3
` + extraLimit + `

[database]
wal_mode = true
vacuum_interval = 86400
message_retention_days = 90

[logging]
max_log_size_mb = 10
max_log_files = 5

[api]
circuit_breaker_threshold = 5
circuit_breaker_timeout = 30
max_retries = 3
retry_backoff_ms = 100
` + extraBlock
}

func TestResolveSecretsPerNetworkPrecedesGlobal(t *testing.T) {
	// The libera network leaves both secrets empty in TOML, so they must be
	// resolved from the environment, with the per-network var winning over the
	// global fallback.
	t.Setenv("LOLO_NICKSERV_PASSWORD", "global-ns")
	t.Setenv("LOLO_SASL_PASSWORD", "global-sasl")
	t.Setenv("LOLO_LIBERA_NICKSERV_PASSWORD", "libera-ns")
	t.Setenv("LOLO_LIBERA_SASL_PASSWORD", "libera-sasl")

	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Networks[0].NickServPassword; got != "libera-ns" {
		t.Fatalf("expected per-network nickserv password to win, got %q", got)
	}
	if got := cfg.Networks[0].SASLPassword; got != "libera-sasl" {
		t.Fatalf("expected per-network sasl password to win, got %q", got)
	}
}

func TestResolveSecretsGlobalFallback(t *testing.T) {
	// With no per-network vars set, the global fallback vars are used.
	t.Setenv("LOLO_NICKSERV_PASSWORD", "global-ns")
	t.Setenv("LOLO_SASL_PASSWORD", "global-sasl")

	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Networks[0].NickServPassword; got != "global-ns" {
		t.Fatalf("expected global nickserv fallback, got %q", got)
	}
	if got := cfg.Networks[0].SASLPassword; got != "global-sasl" {
		t.Fatalf("expected global sasl fallback, got %q", got)
	}
}

func TestResolveSecretsTOMLValueNotOverridden(t *testing.T) {
	// A secret present in the TOML config must win over any env var.
	t.Setenv("LOLO_LIBERA_NICKSERV_PASSWORD", "env-ns")
	t.Setenv("LOLO_NICKSERV_PASSWORD", "global-ns")

	path := writeConfig(t, multiNetworkConfig(networkBlockWithAuth("libera", "irc.libera.chat", "toml-ns", "")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}

	if got := cfg.Networks[0].NickServPassword; got != "toml-ns" {
		t.Fatalf("expected TOML nickserv value to be preserved, got %q", got)
	}
}

func TestValidateSecretsRequiredMissingErrors(t *testing.T) {
	// nickserv_required = true with no resolvable secret must fail with a clear
	// error that names the env vars but never prints a value.
	path := writeConfig(t, multiNetworkConfig(networkBlockRequiredNickServ("libera", "irc.libera.chat")))

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for missing required nickserv password")
	}
	msg := err.Error()
	if !strings.Contains(msg, "LOLO_LIBERA_NICKSERV_PASSWORD") || !strings.Contains(msg, "LOLO_NICKSERV_PASSWORD") {
		t.Fatalf("expected error to name the env vars, got %v", err)
	}
}

func TestValidateSecretsRequiredResolvedFromEnvPasses(t *testing.T) {
	// When the required secret is resolvable from the environment, load succeeds.
	t.Setenv("LOLO_LIBERA_NICKSERV_PASSWORD", "resolved")

	path := writeConfig(t, multiNetworkConfig(networkBlockRequiredNickServ("libera", "irc.libera.chat")))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}
	if got := cfg.Networks[0].NickServPassword; got != "resolved" {
		t.Fatalf("expected resolved nickserv password, got %q", got)
	}
}

func TestDefaultConfigDoesNotTriggerRequiredSecretError(t *testing.T) {
	// The shipped default config sets sasl_username but no passwords and does
	// not opt in to required secrets, so it must validate without any secrets
	// present in the environment (behavioral parity).
	if err := validate(DefaultConfig()); err != nil {
		t.Fatalf("default config failed validation: %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bot.toml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func networkBlock(id, address string) string {
	return `
[[networks]]
id = "` + id + `"
address = "` + address + `"
port = 6697
tls = true
nickname = "Lolo"
username = "lolo"
realname = "Lolo IRC Bot"
max_message_length = 400
channels = ["#mathizen"]
`
}

func networkBlockWithAuth(id, address, nickservPassword, saslPassword string) string {
	return `
[[networks]]
id = "` + id + `"
address = "` + address + `"
port = 6697
tls = true
nickname = "Lolo"
username = "lolo"
realname = "Lolo IRC Bot"
max_message_length = 400
sasl_username = "Lolo"
sasl_password = "` + saslPassword + `"
nickserv_password = "` + nickservPassword + `"
channels = ["#mathizen"]
`
}

func networkBlockRequiredNickServ(id, address string) string {
	return `
[[networks]]
id = "` + id + `"
address = "` + address + `"
port = 6697
tls = true
nickname = "Lolo"
username = "lolo"
realname = "Lolo IRC Bot"
max_message_length = 400
nickserv_required = true
channels = ["#mathizen"]
`
}

func multiNetworkConfig(networks string) string {
	return networks + `
[bot]
command_prefix = "!"
api_endpoint = "http://localhost:8000"
api_timeout = 240

[limits]
rate_limit_messages = 1
rate_limit_window = 1
max_message_queue = 100
reconnect_delay_min = 5
reconnect_delay_max = 300
command_cooldown = 3

[database]
wal_mode = true
vacuum_interval = 86400
message_retention_days = 90

[logging]
max_log_size_mb = 10
max_log_files = 5

[api]
circuit_breaker_threshold = 5
circuit_breaker_timeout = 30
max_retries = 3
retry_backoff_ms = 100
`
}
