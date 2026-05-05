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
