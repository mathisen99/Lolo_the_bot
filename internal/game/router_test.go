package game

import (
	"context"
	"testing"
	"time"
)

func TestRoutePMRejectHelpIsRateLimitedAndAlwaysTerminal(t *testing.T) {
	now := time.Unix(100, 0)
	router := NewRouter(RouterConfig{
		NetworkID: "libera", Prefix: "!", Command: "avenger",
		PMRejectMode: "help", MaxInputBytes: 32, HelpInterval: time.Minute,
	}, RouterDependencies{Now: func() time.Time { return now }})
	input := RouteInput{NetworkID: "libera", Nick: "alice", Message: "hello", IsPM: true, Prefix: "!", ReceivedAt: now}

	first, err := router.RoutePM(context.Background(), input)
	if err != nil || !first.Handled || first.Kind != PMRouteRejected || len(first.Deliveries) != 1 {
		t.Fatalf("first rejection = %#v, %v", first, err)
	}
	second, err := router.RoutePM(context.Background(), input)
	if err != nil || !second.Handled || second.Kind != PMRouteRejected || len(second.Deliveries) != 0 {
		t.Fatalf("rate-limited rejection = %#v, %v", second, err)
	}

	silent := NewRouter(RouterConfig{
		NetworkID: "libera", Prefix: "!", Command: "avenger",
		PMRejectMode: "silent", MaxInputBytes: 32,
	}, RouterDependencies{Now: func() time.Time { return now }})
	out, err := silent.RoutePM(context.Background(), input)
	if err != nil || !out.Handled || out.Kind != PMRouteRejected || len(out.Deliveries) != 0 {
		t.Fatalf("silent rejection = %#v, %v", out, err)
	}
}

func TestExactNamespaceRequiresConfiguredPrefixAndTokenBoundary(t *testing.T) {
	tests := []struct {
		message, prefix, name, wantRemainder string
		want                                 bool
	}{
		{message: "$verify sword fish", prefix: "$", name: "verify", wantRemainder: "sword fish", want: true},
		{message: "$VERIFY sword fish", prefix: "$", name: "verify", wantRemainder: "sword fish", want: true},
		{message: "!verify sword fish", prefix: "$", name: "verify", want: false},
		{message: "$verifyx sword fish", prefix: "$", name: "verify", want: false},
		{message: "$avenger start", prefix: "$", name: "avenger", wantRemainder: "start", want: true},
	}
	for _, tc := range tests {
		got, ok := exactNamespace(tc.message, tc.prefix, tc.name)
		if ok != tc.want || got != tc.wantRemainder {
			t.Fatalf("exactNamespace(%q, %q, %q) = %q, %v", tc.message, tc.prefix, tc.name, got, ok)
		}
	}
}

func TestRoutePMFeatureOffFallsThroughExceptReservedGameNamespace(t *testing.T) {
	router := NewRouter(RouterConfig{
		NetworkID: "libera", Prefix: "!", Command: "avenger", Enabled: false,
	}, RouterDependencies{})

	out, err := router.RoutePMFeatureOff(RouteInput{
		NetworkID: "libera", Nick: "alice", Message: "!echo hello", IsPM: true,
	})
	if err != nil || out.Handled {
		t.Fatalf("non-game PM during rollback = %#v, %v; want pre-feature fallthrough", out, err)
	}

	out, err = router.RoutePMFeatureOff(RouteInput{
		NetworkID: "libera", Nick: "alice", Message: "!avenger start", IsPM: true,
	})
	if err != nil || !out.Handled || out.Kind != PMRouteRejected {
		t.Fatalf("disabled game PM = %#v, %v", out, err)
	}
	if len(out.Deliveries) != 1 || len(out.Deliveries[0].Lines) != 1 || out.Deliveries[0].Lines[0] != "Game is unavailable." {
		t.Fatalf("disabled game delivery = %#v", out.Deliveries)
	}
}
