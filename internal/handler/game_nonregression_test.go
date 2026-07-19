package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/game"
)

func installDisabledGameRouter(env *handlerTestEnv) {
	env.handler.gameRouter = game.NewRouter(
		game.RouterConfig{
			NetworkID: "libera", Prefix: "!", Command: "avenger",
			Enabled: false, PMEnabled: true, PMRejectMode: "help", MaxInputBytes: 512,
		},
		game.RouterDependencies{
			Users:  env.handler.userManager,
			Verify: commands.NewVerifyCommand(env.handler.userManager, env.db),
		},
	)
}

func TestFeatureOffRestoresNonGamePMBehaviorAndKeepsVerifyIsolated(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()
	installDisabledGameRouter(env)
	env.apiClient.commandResponses["echo"] = "echo-ok"
	env.apiClient.commandResponses["avenger"] = "generic-game-mutation-must-not-run"

	responses, err := env.handler.HandleMessage(
		context.Background(), "guest", "guest@untrusted", "", "!echo hello", true, nil,
	)
	if err != nil || len(responses) != 1 || responses[0] != "echo-ok" {
		t.Fatalf("feature-off non-game PM = %v, %v", responses, err)
	}

	responses, err = env.handler.HandleMessage(
		context.Background(), "guest", "guest@untrusted", "", "!avenger start", true, nil,
	)
	if err != nil || len(responses) != 1 || responses[0] != "Game is unavailable." {
		t.Fatalf("feature-off game PM = %v, %v", responses, err)
	}

	if err := env.handler.userManager.SetOwnerPassword("rollout-secret"); err != nil {
		t.Fatalf("SetOwnerPassword: %v", err)
	}
	responses, err = env.handler.HandleMessage(
		context.Background(), "guest", "guest@untrusted", "", "!verify rollout-secret", true, nil,
	)
	if err != nil || len(responses) != 1 || responses[0] != "Owner verified! You are now the bot owner." {
		t.Fatalf("feature-off verify = %v, %v", responses, err)
	}
	messages, queryErr := env.db.QueryMessages(&database.MessageFilter{Network: "libera", Nick: "guest", Limit: 20})
	if queryErr != nil {
		t.Fatalf("QueryMessages: %v", queryErr)
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "rollout-secret") {
			t.Fatal("verification password entered ordinary message persistence")
		}
	}
	if size := env.handler.userManager.GetWhoisCacheSize(); size != 0 {
		t.Fatalf("unregistered feature-off routes populated WHOIS state: %d", size)
	}
}

func TestFeatureOffPreservesRepresentativeChannelBehavior(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()
	installDisabledGameRouter(env)
	ctx := context.Background()

	env.dispatcher.SetChannelPrefix("#smoke", "$")
	env.apiClient.commandResponses["echo"] = "dynamic-ok"
	env.apiClient.streamingResponses["stream_example"] = []*APIResponse{
		{RequestID: "stream-smoke", Status: "success", Message: "chunk-one", Streaming: true},
		{RequestID: "stream-smoke", Status: "success", Message: "chunk-two", Streaming: false},
	}
	env.handler.commandMetadata["stream_example"] = &CommandMetadata{
		Name: "stream_example", RequiredPermission: "any", Timeout: 30, Streaming: true,
	}
	env.apiClient.mentionResponse = "mention-ok"

	responses, err := env.handler.HandleMessage(ctx, "guest", "", "#smoke", "$topic core", false, nil)
	if err != nil || len(responses) != 1 || responses[0] != "topic:core" {
		t.Fatalf("core command smoke = %v, %v", responses, err)
	}
	responses, err = env.handler.HandleMessage(ctx, "guest", "", "#smoke", "!topic old-prefix", false, nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("old prefix unexpectedly executed = %v, %v", responses, err)
	}
	responses, err = env.handler.HandleMessage(ctx, "guest", "", "#smoke", "$echo hello", false, nil)
	if err != nil || len(responses) != 1 || responses[0] != "dynamic-ok" {
		t.Fatalf("dynamic command smoke = %v, %v", responses, err)
	}
	responses, err = env.handler.HandleMessage(ctx, "guest", "", "#smoke", "$stream_example 2", false, nil)
	if err != nil || len(responses) != 2 || responses[0] != "chunk-one" || responses[1] != "chunk-two" {
		t.Fatalf("streaming command smoke = %v, %v", responses, err)
	}
	responses, err = env.handler.handleMention(ctx, "guest", "", "#smoke", "Lolo hello", nil)
	if err != nil || len(responses) != 1 || responses[0] != "mention-ok" || env.apiClient.mentionPrefix != "$" {
		t.Fatalf("channel mention smoke = %v, prefix=%q, err=%v", responses, env.apiClient.mentionPrefix, err)
	}

	if err := env.handler.userManager.AddUser("ignored", "ignored@host", database.LevelIgnored); err != nil {
		t.Fatalf("AddUser ignored: %v", err)
	}
	responses, err = env.handler.HandleMessage(ctx, "ignored", "ignored@host", "#smoke", "$echo hidden", false, nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("ignored channel command was not suppressed = %v, %v", responses, err)
	}
	responses, err = env.handler.handleMention(ctx, "ignored", "ignored@host", "#smoke", "Lolo hidden", nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("ignored channel mention was not suppressed = %v, %v", responses, err)
	}

	if err := env.db.SetChannelStateForNetwork("libera", "#disabled", false); err != nil {
		t.Fatalf("SetChannelStateForNetwork: %v", err)
	}
	responses, err = env.handler.HandleMessage(ctx, "guest", "", "#disabled", "!topic hidden", false, nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("disabled-channel core command was not suppressed = %v, %v", responses, err)
	}
	responses, err = env.handler.HandleMessage(ctx, "guest", "", "#disabled", "!echo hidden", false, nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("disabled-channel dynamic command was not suppressed = %v, %v", responses, err)
	}
	responses, err = env.handler.handleMention(ctx, "guest", "", "#disabled", "Lolo hidden", nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("disabled-channel mention was not suppressed = %v, %v", responses, err)
	}
	if size := env.handler.userManager.GetWhoisCacheSize(); size != 0 {
		t.Fatalf("unregistered channel routes populated WHOIS state: %d", size)
	}
}
