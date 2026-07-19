package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yourusername/lolo/internal/database"
)

func testContinuationKey(network, value string) ContinuationKey {
	return ContinuationKey{NetworkID: network, IdentityKind: IdentityUnregistered, IdentityValue: value}
}

func statusBinding(contextID string, revision int64) ContinuationBinding {
	return ContinuationBinding{Input: "status", Kind: "action", Action: "status", MenuContextID: contextID, StateRevision: revision}
}

func choiceBinding(contextID, token string, revision int64) ContinuationBinding {
	return ContinuationBinding{
		Input: token, Kind: "choice", Action: "attack",
		Arguments: ActionArguments{TargetID: "dock_raider"}, ChoiceToken: token,
		MenuContextID: contextID, StateRevision: revision,
	}
}

func TestContinuationRegistryAcceptsOnlyCurrentExactBinding(t *testing.T) {
	now := time.Unix(1000, 0)
	registry := NewContinuationRegistry(ContinuationRegistryConfig{
		Capacity: 4, MaxBindings: 4, MaxTTL: 15 * time.Minute,
		Now: func() time.Time { return now },
	})
	alice := testContinuationKey("libera", "alice")
	bindings := []ContinuationBinding{
		statusBinding("ctx-one", 7),
		choiceBinding("ctx-one", "c-abcdef", 7),
	}
	if err := registry.ReplaceForNick(alice, "alice", "ctx-one", bindings, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if binding, status := registry.Resolve(alice, "  STATUS\t", now); status != ContinuationCurrent || binding.Action != "status" {
		t.Fatalf("named current resolve = %#v, %v", binding, status)
	}
	if _, status := registry.Resolve(alice, "c-abcdef", now); status != ContinuationCurrent {
		t.Fatalf("choice current status = %v", status)
	}
	for _, input := range []string{"1", "status now", "stat", "c-ABCDEF", "c-abcdeg"} {
		if _, status := registry.Resolve(alice, input, now); status == ContinuationCurrent {
			t.Fatalf("non-exact input %q was accepted", input)
		}
	}
	if _, status := registry.Resolve(testContinuationKey("rizon", "alice"), "status", now); status != ContinuationUnknown {
		t.Fatalf("cross-network status = %v", status)
	}
	if _, status := registry.Resolve(testContinuationKey("libera", "bob"), "status", now); status != ContinuationUnknown {
		t.Fatalf("cross-identity status = %v", status)
	}
}

func TestContinuationRegistryAtomicReplacementExpiryAndCapacity(t *testing.T) {
	now := time.Unix(2000, 0)
	registry := NewContinuationRegistry(ContinuationRegistryConfig{
		Capacity: 1, MaxBindings: 2, MaxTTL: time.Minute,
		Now: func() time.Time { return now },
	})
	alice := testContinuationKey("libera", "alice")
	if err := registry.Replace(alice, "ctx-one", []ContinuationBinding{statusBinding("ctx-one", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatalf("initial Replace: %v", err)
	}

	bad := []ContinuationBinding{
		statusBinding("ctx-two", 2),
		{Input: "1", Kind: "action", Action: "status", MenuContextID: "ctx-two", StateRevision: 2},
	}
	if err := registry.Replace(alice, "ctx-two", bad, now.Add(time.Minute)); err == nil {
		t.Fatal("invalid replacement succeeded")
	}
	if _, status := registry.Resolve(alice, "status", now); status != ContinuationCurrent {
		t.Fatalf("invalid replacement was not atomic, status=%v", status)
	}

	if err := registry.Replace(alice, "ctx-two", []ContinuationBinding{{Input: "help", Kind: "action", Action: "help", MenuContextID: "ctx-two", StateRevision: 2}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("superseding Replace: %v", err)
	}
	if _, status := registry.Resolve(alice, "status", now); status != ContinuationStale {
		t.Fatalf("superseded input status = %v, want stale", status)
	}
	if _, status := registry.Resolve(alice, "help", now.Add(time.Minute)); status != ContinuationExpired {
		t.Fatalf("expiry boundary status = %v, want expired", status)
	}

	// Once expired capacity is reclaimed. When it is not reclaimable, callers
	// can ignore this error and prefixed game commands remain independent.
	if err := registry.Replace(testContinuationKey("libera", "bob"), "ctx-three", []ContinuationBinding{statusBinding("ctx-three", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatalf("expired capacity was not reclaimed: %v", err)
	}
	if err := registry.Replace(alice, "ctx-four", []ContinuationBinding{statusBinding("ctx-four", 1)}, now.Add(time.Minute)); !errors.Is(err, ErrContinuationCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
}

func TestContinuationLifecycleInvalidationAndObservedNickTransfer(t *testing.T) {
	now := time.Unix(3000, 0)
	newRegistry := func() *ContinuationRegistry {
		return NewContinuationRegistry(ContinuationRegistryConfig{Capacity: 5, MaxBindings: 2, MaxTTL: time.Minute, Now: func() time.Time { return now }})
	}
	registry := newRegistry()
	cases := NewCaseMappingStore()
	lifecycle := NewLifecycleManager(cases, registry, nil)
	oldKey := testContinuationKey("libera", "alice")
	newKey := testContinuationKey("libera", "alice_")
	if err := registry.ReplaceForNick(oldKey, "alice", "ctx-one", []ContinuationBinding{statusBinding("ctx-one", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	lifecycle.OnObservedNickChange("libera", "Alice", "Alice_")
	if _, status := registry.Resolve(oldKey, "status", now); status == ContinuationCurrent {
		t.Fatal("old nickname retained transferred context")
	}
	if _, status := registry.Resolve(newKey, "status", now); status != ContinuationCurrent {
		t.Fatalf("observed destination status = %v", status)
	}

	lifecycle.OnUserQuit("libera", "Alice_")
	if _, status := registry.Resolve(newKey, "status", now); status == ContinuationCurrent {
		t.Fatal("QUIT did not invalidate context")
	}
	registeredKey := ContinuationKey{NetworkID: "libera", IdentityKind: IdentityRegistered, IdentityValue: "7"}
	if err := registry.ReplaceForNick(registeredKey, "registered", "ctx-registered", []ContinuationBinding{statusBinding("ctx-registered", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	lifecycle.OnUserQuit("libera", "Registered")
	if _, status := registry.Resolve(registeredKey, "status", now); status == ContinuationCurrent {
		t.Fatal("registered QUIT alias did not invalidate context")
	}
	if err := registry.ReplaceForNick(newKey, "alice_", "ctx-two", []ContinuationBinding{statusBinding("ctx-two", 2)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	lifecycle.OnConnectionClosed("libera")
	if _, status := registry.Resolve(newKey, "status", now); status == ContinuationCurrent {
		t.Fatal("disconnect did not invalidate context")
	}
	if err := registry.ReplaceForNick(newKey, "alice_", "ctx-three", []ContinuationBinding{statusBinding("ctx-three", 3)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	lifecycle.OnConnectionEstablished("libera")
	if _, status := registry.Resolve(newKey, "status", now); status == ContinuationCurrent {
		t.Fatal("reconnect did not invalidate context")
	}
	if _, status := newRegistry().Resolve(newKey, "status", now); status != ContinuationUnknown {
		t.Fatalf("restart registry status = %v", status)
	}
}

func TestObservedNickDestinationConflictPreservesSessionsAndNeverInfersReuse(t *testing.T) {
	now := time.Unix(4000, 0)
	registry := NewContinuationRegistry(ContinuationRegistryConfig{Capacity: 4, MaxBindings: 2, MaxTTL: time.Minute, Now: func() time.Time { return now }})
	oldKey := testContinuationKey("libera", "alice")
	newKey := testContinuationKey("libera", "bob")
	if err := registry.ReplaceForNick(oldKey, "alice", "ctx-old", []ContinuationBinding{statusBinding("ctx-old", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := registry.ReplaceForNick(newKey, "bob", "ctx-new", []ContinuationBinding{{Input: "help", Kind: "action", Action: "help", MenuContextID: "ctx-new", StateRevision: 9}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	// These stand in for persistent sessions; registry conflict handling must
	// not merge, overwrite, or delete either one.
	sessions := map[ContinuationKey]string{oldKey: "old-save", newKey: "destination-save"}
	if err := registry.Transfer("libera", "alice", "bob"); !errors.Is(err, ErrIdentityAmbiguous) {
		t.Fatalf("Transfer conflict = %v", err)
	}
	if sessions[oldKey] != "old-save" || sessions[newKey] != "destination-save" || len(sessions) != 2 {
		t.Fatalf("destination conflict changed sessions: %#v", sessions)
	}
	if _, status := registry.Resolve(oldKey, "status", now); status == ContinuationCurrent {
		t.Fatal("source context survived identity conflict")
	}
	if _, status := registry.Resolve(newKey, "help", now); status == ContinuationCurrent {
		t.Fatal("destination context survived identity conflict")
	}

	// Later reuse of the old nickname is not an observed transfer and cannot
	// inherit the destination context.
	if err := registry.Transfer("libera", "alice", "charlie"); err != nil {
		t.Fatalf("missing-source transfer should be a no-op: %v", err)
	}
	if _, status := registry.Resolve(testContinuationKey("libera", "charlie"), "status", now); status != ContinuationUnknown {
		t.Fatalf("later nickname reuse inferred a transfer: %v", status)
	}
}

func TestContinuationCapacityDoesNotBlockPrefixedGameCommand(t *testing.T) {
	now := time.Unix(4500, 0)
	registry := NewContinuationRegistry(ContinuationRegistryConfig{Capacity: 1, MaxBindings: 1, MaxTTL: time.Minute, Now: func() time.Time { return now }})
	if err := registry.Replace(testContinuationKey("libera", "occupied"), "ctx-full", []ContinuationBinding{statusBinding("ctx-full", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	manager := &identityManagerSpy{users: map[string]*database.User{}, registered: map[string]bool{}}
	resolver := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) {
			panic("unregistered command invoked registered identity policy")
		},
	))
	calls := 0
	router := NewRouter(RouterConfig{NetworkID: "libera", Prefix: "!", Command: "avenger", Enabled: true, PMEnabled: true, PMRejectMode: "silent", MaxInputBytes: 64}, RouterDependencies{
		Users: manager, PMState: staticPMState(true), Identities: resolver, Continuations: registry, Now: func() time.Time { return now },
		Game: PMGameHandlerFunc(func(_ context.Context, input PMGameInput) ([]Delivery, error) {
			calls++
			if input.Identity != (SessionIdentity{Kind: IdentityUnregistered, Value: "guest"}) {
				t.Fatalf("game identity = %#v", input.Identity)
			}
			return nil, nil
		}),
	})
	out, err := router.RoutePM(context.Background(), RouteInput{NetworkID: "libera", Nick: "Guest", Hostmask: "must-not-cross@host", Message: "!avenger status", IsPM: true, ReceivedAt: now})
	if err != nil || out.Kind != PMRouteGameCommand || calls != 1 {
		t.Fatalf("prefixed route under full capacity = %#v err=%v calls=%d", out, err, calls)
	}
}

func TestRouterRegisteredAmbiguityFailsClosedAndQuitInvalidates(t *testing.T) {
	now := time.Unix(5000, 0)
	manager := &identityManagerSpy{
		users:      map[string]*database.User{"Alice": {ID: 7, Nick: "Alice", Level: database.LevelNormal}},
		registered: map[string]bool{"Alice": true},
	}
	gameCalls := 0
	gameHandler := PMGameHandlerFunc(func(context.Context, PMGameInput) ([]Delivery, error) {
		gameCalls++
		return nil, nil
	})
	registry := NewContinuationRegistry(ContinuationRegistryConfig{Capacity: 2, MaxBindings: 2, MaxTTL: time.Minute, Now: func() time.Time { return now }})
	ambiguous := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) { return false, nil },
	))
	router := NewRouter(RouterConfig{NetworkID: "libera", Prefix: "!", Command: "avenger", Enabled: true, PMEnabled: true, PMRejectMode: "silent", MaxInputBytes: 64}, RouterDependencies{
		Users: manager, PMState: staticPMState(true), Game: gameHandler, Identities: ambiguous, Continuations: registry, Now: func() time.Time { return now },
	})
	out, err := router.RoutePM(context.Background(), RouteInput{NetworkID: "libera", Nick: "Alice", Hostmask: "untrusted@host", Message: "!avenger status", IsPM: true, ReceivedAt: now})
	if err != nil || !out.Handled || out.Kind != PMRouteRejected || gameCalls != 0 {
		t.Fatalf("ambiguous route = %#v err=%v calls=%d", out, err, gameCalls)
	}

	unregisteredResolver := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) {
			panic("unregistered quit invoked WHOIS/NickServ policy")
		},
	))
	guestKey := testContinuationKey("libera", "guest")
	if err := registry.ReplaceForNick(guestKey, "guest", "ctx-quit", []ContinuationBinding{statusBinding("ctx-quit", 1)}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	router = NewRouter(RouterConfig{NetworkID: "libera", Prefix: "!", Command: "avenger", Enabled: true, PMEnabled: true, PMRejectMode: "silent", MaxInputBytes: 64}, RouterDependencies{
		Users: manager, PMState: staticPMState(true), Game: gameHandler, Identities: unregisteredResolver, Continuations: registry, Now: func() time.Time { return now },
	})
	out, err = router.RoutePM(context.Background(), RouteInput{NetworkID: "libera", Nick: "Guest", Hostmask: "must-not-cross@host", Message: "!avenger quit", IsPM: true, ReceivedAt: now})
	if err != nil || out.Kind != PMRouteGameCommand || gameCalls != 1 {
		t.Fatalf("quit route = %#v err=%v calls=%d", out, err, gameCalls)
	}
	if _, status := registry.Resolve(guestKey, "status", now); status == ContinuationCurrent {
		t.Fatal("prefixed quit did not invalidate current context")
	}
}

// Small adapters keep the tests on the real router without mocks of game logic.
type PMGameHandlerFunc func(context.Context, PMGameInput) ([]Delivery, error)

func (f PMGameHandlerFunc) HandlePMGame(ctx context.Context, input PMGameInput) ([]Delivery, error) {
	return f(ctx, input)
}

type staticPMState bool

func (s staticPMState) GetPMState() (bool, error) { return bool(s), nil }
