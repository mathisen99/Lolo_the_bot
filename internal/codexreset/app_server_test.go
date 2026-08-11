package codexreset

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadRPCResponseSkipsNotifications(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{}}}` + "\n" +
			`{"id":1,"result":{"userAgent":"test"}}` + "\n",
	))

	envelope, err := readRPCResponse(context.Background(), scanner, 1)
	if err != nil {
		t.Fatalf("readRPCResponse: %v", err)
	}
	if !strings.Contains(string(envelope.Result), "userAgent") {
		t.Fatalf("unexpected result: %s", envelope.Result)
	}
}

func TestReadRPCResponseReturnsProtocolError(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(`{"id":"1","error":{"code":-32000,"message":"not authenticated"}}` + "\n"))

	_, err := readRPCResponse(context.Background(), scanner, 1)
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("error = %v, want authentication error", err)
	}
}

func TestNormalizeRateLimitsPrefersMultiBucketView(t *testing.T) {
	resetAt := int64(1234)
	result := rateLimitsResult{
		RateLimits: &RateLimitSnapshot{LimitID: "legacy"},
		RateLimitsByLimitID: map[string]RateLimitSnapshot{
			"codex": {Primary: &RateLimitWindow{UsedPercent: 42, ResetsAt: &resetAt}},
		},
	}

	normalized := normalizeRateLimits(result)
	if len(normalized) != 1 || normalized["codex"].LimitID != "codex" {
		t.Fatalf("normalized rate limits = %#v, want codex multi-bucket view", normalized)
	}
	if normalized["codex"].Primary == nil || normalized["codex"].Primary.UsedPercent != 42 {
		t.Fatalf("normalized primary window = %#v, want usedPercent 42", normalized["codex"].Primary)
	}
}

func TestLiveAppServerAccountRead(t *testing.T) {
	if os.Getenv("LOLO_TEST_CODEX_APP_SERVER") != "1" {
		t.Skip("set LOLO_TEST_CODEX_APP_SERVER=1 to test the local Codex login")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	snapshot, err := (&AppServerSource{CodexPath: "/usr/bin/codex"}).Read(ctx)
	if err != nil {
		t.Fatalf("live Codex app-server account read failed: %v", err)
	}
	if len(snapshot.RateLimits) == 0 {
		t.Fatal("live Codex app-server account read returned no rate-limit buckets")
	}
}

func TestLiveWatcherDryRunStaysSilentForStableAccountState(t *testing.T) {
	if os.Getenv("LOLO_TEST_CODEX_APP_SERVER") != "1" {
		t.Skip("set LOLO_TEST_CODEX_APP_SERVER=1 to dry-run the watcher against the local Codex login")
	}

	channels := []string{"#mathizen", "#mathizen.net", "##llm"}
	sender := &recordingSender{}
	watcher := newWithSource(
		testOptions(t.TempDir()+"/state.json", channels),
		sender,
		discardLogger{},
		&AppServerSource{CodexPath: "/usr/bin/codex"},
	)
	state := persistedState{Version: stateVersion}

	// The first real read establishes a baseline; a second unchanged real read
	// must remain silent. The recording sender has no IRC connection, so even a
	// regression can only fail this test rather than message a channel.
	watcher.check(context.Background(), &state)
	watcher.check(context.Background(), &state)

	for _, channel := range channels {
		if got := sender.count(channel); got != 0 {
			t.Fatalf("dry run produced %d notification(s) for %s, want 0", got, channel)
		}
	}
}
