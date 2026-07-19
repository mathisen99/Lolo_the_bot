package config

import (
	"strings"
	"testing"
)

func TestGameConfigDefaultsAreSafeAndDisabled(t *testing.T) {
	cfg := DefaultConfig().Game
	if cfg.Enabled || cfg.ChannelPlayEnabled || cfg.MilestoneAnnouncementsEnabled || cfg.AdultContentEnabled || cfg.RealPersonContentEnabled || cfg.AIEnhancementEnabled {
		t.Fatalf("dangerous game capabilities must default off: %#v", cfg)
	}
	if !cfg.PMEnabled || cfg.StandardContentProfile != "standard" || cfg.DatabasePath != "data/game.db" {
		t.Fatalf("unexpected safe defaults: %#v", cfg)
	}
	if cfg.ConfigRevision < 1 || cfg.ContentPolicyRevision < 1 {
		t.Fatal("configuration revisions must be positive")
	}
}

func TestLoadGameConfigPreservesExplicitFalseAndDefaults(t *testing.T) {
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat"))+`
[game]
enabled = true
pm_enabled = false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Game.Enabled {
		t.Fatalf("valid explicit enable should be retained: %v", cfg.Game.ValidationErrors())
	}
	if cfg.Game.PMEnabled {
		t.Fatal("explicit pm_enabled=false was overwritten")
	}
	if cfg.Game.DatabasePath != DefaultGameDatabasePath || cfg.Game.MaxInputBytes != 512 {
		t.Fatal("omitted game fields did not receive defaults")
	}
}

func TestInvalidGameConfigDegradesOnlyGame(t *testing.T) {
	path := writeConfig(t, multiNetworkConfig(networkBlock("libera", "irc.libera.chat"))+`
[game]
enabled = true
database_path = "../stolen.db"
max_input_bytes = 99999
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("game validation must not fail global config: %v", err)
	}
	if cfg.Game.Enabled {
		t.Fatal("invalid game config must force game disabled")
	}
	joined := strings.Join(cfg.Game.ValidationErrors(), " ")
	if !strings.Contains(joined, "game.database_path") || !strings.Contains(joined, "game.max_input_bytes") {
		t.Fatalf("expected field-specific game errors, got %q", joined)
	}
	if cfg.Bot.APIEndpoint == "" || len(cfg.Networks) != 1 {
		t.Fatal("unrelated configuration was degraded")
	}
}

func TestGameConfigRejectsDuplicateOrUnknownChannelPairs(t *testing.T) {
	cfg := DefaultGameConfig()
	cfg.ChannelAllowlist = []GameChannel{{Network: "other", Channel: "#play"}, {Network: "other", Channel: "#PLAY"}}
	problems := validateGameConfig(cfg, []NetworkConfig{{ID: "libera"}})
	joined := strings.Join(problems, " ")
	if !strings.Contains(joined, "unknown network") || !strings.Contains(joined, "duplicate") {
		t.Fatalf("unexpected validation: %q", joined)
	}
}

func TestGameBoundaryLimitsUseStricterValues(t *testing.T) {
	goLimits := GameBoundaryLimits{512, 4, 6, 600, 10}
	pythonLimits := GameBoundaryLimits{256, 8, 4, 800, 5}
	got := goLimits.Reconcile(pythonLimits)
	want := (GameBoundaryLimits{256, 4, 4, 600, 5})
	if got != want {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
