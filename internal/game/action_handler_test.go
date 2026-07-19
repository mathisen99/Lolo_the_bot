package game

import (
	"context"
	"sync"
	"testing"
	"time"
)

type actionClientFunc func(context.Context, ActionRequest, time.Duration) (*ActionResponse, error)

func (f actionClientFunc) SendGameAction(ctx context.Context, request ActionRequest, timeout time.Duration) (*ActionResponse, error) {
	return f(ctx, request, timeout)
}

func testPMInput(command string) PMGameInput {
	return PMGameInput{NetworkID: "libera", Identity: SessionIdentity{Kind: IdentityUnregistered, Value: "alice"}, Nick: "Alice", Prefix: "!", CommandText: command}
}

func successfulReadOnly(request ActionRequest) *ActionResponse {
	return &ActionResponse{RequestID: request.RequestID, Status: "success", ResultCategory: "menu", StateRevision: request.ExpectedStateRevision, StateChanged: false}
}

func TestActionHandlerUsesSeparateAIAndDirectBuckets(t *testing.T) {
	var mu sync.Mutex
	var actions []string
	client := actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		mu.Lock()
		actions = append(actions, request.Action.Name)
		mu.Unlock()
		return successfulReadOnly(request), nil
	})
	handler := NewActionHandler(client, nil, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 64, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1, MaxPending: 2,
		DirectBurst: 2, DirectWindow: time.Minute,
		AIRequests: 1, AIBurst: 1, AIWindow: time.Hour, AIEnabled: true,
	})
	if _, err := handler.HandlePMGame(context.Background(), testPMInput("ask go north")); err != nil {
		t.Fatal(err)
	}
	limited, err := handler.HandlePMGame(context.Background(), testPMInput("ask go south"))
	if err != nil || len(limited) != 1 {
		t.Fatalf("second AI admission = %v, %v", limited, err)
	}
	if _, err := handler.HandlePMGame(context.Background(), testPMInput("status")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 2 || actions[0] != "ask" || actions[1] != "status" {
		t.Fatalf("AI exhaustion consumed direct bucket or invoked endpoint: %v", actions)
	}
}

func TestActionHandlerRejectsWhenPendingCapacityIsFull(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int
	client := actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		calls++
		close(entered)
		<-release
		return successfulReadOnly(request), nil
	})
	handler := NewActionHandler(client, nil, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 64, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1, MaxPending: 1,
		DirectBurst: 4, DirectWindow: time.Minute, AIRequests: 1, AIBurst: 1, AIWindow: time.Hour,
	})
	done := make(chan struct{})
	go func() { _, _ = handler.HandlePMGame(context.Background(), testPMInput("status")); close(done) }()
	<-entered
	rejected, err := handler.HandlePMGame(context.Background(), testPMInput("inventory"))
	if err != nil || len(rejected) != 1 {
		t.Fatalf("pending rejection = %v, %v", rejected, err)
	}
	if calls != 1 {
		t.Fatalf("full pending queue invoked endpoint %d times", calls)
	}
	close(release)
	<-done
}

func TestActionHandlerBoundsInputAndNormalizesMalformedActionsToAuthoredHelp(t *testing.T) {
	calls := 0
	client := actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		calls++
		if request.Action.Name != "help" || !request.Action.Arguments.Fallback {
			t.Fatalf("malformed action was not normalized to authored help: %#v", request.Action)
		}
		return successfulReadOnly(request), nil
	})
	handler := NewActionHandler(client, nil, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 8, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1,
	})
	for _, command := range []string{"status now", "attack", "123456789"} {
		deliveries, err := handler.HandlePMGame(context.Background(), testPMInput(command))
		if err != nil {
			t.Fatalf("input %q returned %v", command, err)
		}
		if command != "attack" && len(deliveries) != 1 {
			t.Fatalf("bounded rejection %q = %v", command, deliveries)
		}
	}
	if calls != 1 {
		t.Fatalf("expected only bounded malformed input to request authored help, got %d calls", calls)
	}
}

func TestActionHandlerDisabledAskNeverCallsEndpoint(t *testing.T) {
	calls := 0
	handler := NewActionHandler(actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		calls++
		return successfulReadOnly(request), nil
	}), nil, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 64, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1,
	})
	deliveries, err := handler.HandlePMGame(context.Background(), testPMInput("ask go north"))
	if err != nil || len(deliveries) != 1 || deliveries[0].Target != DeliveryPM {
		t.Fatalf("disabled ask = %v, %v", deliveries, err)
	}
	if calls != 0 {
		t.Fatalf("disabled ask invoked endpoint %d times", calls)
	}
}

func TestActionHandlerUsesCurrentRevisionAndAcceptsPrefixedOpaqueChoice(t *testing.T) {
	now := time.Date(2026, 5, 1, 2, 3, 4, 0, time.UTC)
	key, err := ContinuationKeyFor("libera", SessionIdentity{Kind: IdentityUnregistered, Value: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewContinuationRegistry(ContinuationRegistryConfig{Now: func() time.Time { return now }})
	if err := registry.Replace(key, "m-current", []ContinuationBinding{
		{Input: "status", Kind: "action", Action: "status", MenuContextID: "m-current", StateRevision: 7},
		{Input: "c-abcdef", Kind: "choice", Action: "travel", Arguments: ActionArguments{DestinationID: "docks"}, ChoiceToken: "c-abcdef", MenuContextID: "m-current", StateRevision: 7},
	}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var requests []ActionRequest
	handler := NewActionHandler(actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		requests = append(requests, request)
		return successfulReadOnly(request), nil
	}), registry, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 64, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1, Now: func() time.Time { return now },
	})
	for _, command := range []string{"status", "c-abcdef"} {
		if _, err := handler.HandlePMGame(context.Background(), testPMInput(command)); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 2 || requests[0].ExpectedStateRevision != 7 || requests[1].ExpectedStateRevision != 7 {
		t.Fatalf("current revision was not retained: %#v", requests)
	}
	if requests[1].Action.Name != "travel" || requests[1].Action.Arguments.DestinationID != "docks" || requests[1].Action.ChoiceToken != "c-abcdef" {
		t.Fatalf("prefixed opaque choice was not resolved exactly: %#v", requests[1].Action)
	}
}
