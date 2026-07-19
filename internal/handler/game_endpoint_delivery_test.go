package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/game"
)

func newDedicatedGameClient(server *httptest.Server) *APIClient {
	client := NewAPIClient(server.URL, time.Second, DefaultHTTPTransportConfig())
	client.ConfigureGameTransport(1, 64*1024)
	return client
}

func testActionRequest() game.ActionRequest {
	requestID := uuid.NewString()
	return game.ActionRequest{
		RequestID: requestID, IdempotencyKey: requestID, NetworkID: "libera",
		Identity:    game.SessionIdentity{Kind: game.IdentityUnregistered, Value: "alice"},
		DisplayNick: "Alice", Source: game.RequestSource{Kind: game.SourcePM, EffectivePrefix: "!"},
		Operation: game.OperationAction, Mode: game.ModeDirect,
		Action:        game.Action{Name: "start"},
		ClientContext: game.ClientContext{ContentPolicyRevision: 1, ConfigurationRevision: 1},
	}
}

func TestDedicatedGameClientPathsAndCallerRequestIDs(t *testing.T) {
	var paths []string
	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		headers = append(headers, r.Header.Get("X-Request-ID"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/game/action":
			var request game.ActionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(game.ActionResponse{RequestID: request.RequestID, Status: "success", ResultCategory: "started", StateRevision: 1, StateChanged: true, Deliveries: []game.Delivery{{Target: game.DeliveryPM, Lines: []string{"ready"}}}})
		case "/game/lifecycle":
			var request game.LifecycleRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(game.LifecycleResponse{RequestID: request.RequestID, Status: "success"})
		case "/game/health":
			_ = json.NewEncoder(w).Encode(game.HealthStatus{Status: "ready", DatabaseAvailable: true, SchemaVersion: 1, MigrationStatus: "current", EngineVersion: "v1", ContentVersion: "standard-v1", ConfigRevision: 1, AIStatus: "disabled"})
		default:
			t.Fatalf("generic or unexpected endpoint reached: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := newDedicatedGameClient(server)

	action := testActionRequest()
	if _, err := client.SendGameAction(context.Background(), action, time.Second); err != nil {
		t.Fatalf("SendGameAction: %v", err)
	}
	lifecycleID := uuid.NewString()
	lifecycle := game.LifecycleRequest{RequestID: lifecycleID, NetworkID: "libera", Operation: game.OperationInvalidate, Identity: action.Identity, ConfigurationRevision: 1}
	if _, err := client.SendGameLifecycle(context.Background(), lifecycle, time.Second); err != nil {
		t.Fatalf("SendGameLifecycle: %v", err)
	}
	healthID := uuid.NewString()
	if _, err := client.CheckGameHealth(context.Background(), healthID, time.Second); err != nil {
		t.Fatalf("CheckGameHealth: %v", err)
	}

	wantPaths := []string{"/game/action", "/game/lifecycle", "/game/health"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	wantIDs := []string{action.RequestID, lifecycleID, healthID}
	if strings.Join(headers, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("request IDs = %v, want %v", headers, wantIDs)
	}
}

func TestDedicatedGameRetryReusesRequestIDAndBody(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var ids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), raw...))
		ids = append(ids, r.Header.Get("X-Request-ID"))
		attempt := len(bodies)
		mu.Unlock()
		if attempt == 1 {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		var request game.ActionRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(game.ActionResponse{RequestID: request.RequestID, Status: "success", ResultCategory: "started", StateRevision: 1, StateChanged: true})
	}))
	defer server.Close()
	client := newDedicatedGameClient(server)
	request := testActionRequest()
	if _, err := client.SendGameAction(context.Background(), request, time.Second); err != nil {
		t.Fatalf("SendGameAction: %v", err)
	}
	if len(ids) != 2 || ids[0] != request.RequestID || ids[1] != request.RequestID || string(bodies[0]) != string(bodies[1]) {
		t.Fatalf("retry changed correlation/body: ids=%v bodiesEqual=%v", ids, len(bodies) == 2 && string(bodies[0]) == string(bodies[1]))
	}
}

func TestGameMalformedResponseIsAtomicAndGenericSpiesStayUntouched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/game/action" {
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		var request game.ActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(game.ActionResponse{
			RequestID: request.RequestID, Status: "success", ResultCategory: "started",
			StateRevision: 1, StateChanged: true,
			Deliveries: []game.Delivery{{Target: game.DeliveryChannel, Lines: []string{"private state"}}},
		})
	}))
	defer server.Close()
	h, _, users, db, cleanup := newPMBoundaryHandler(t, true, false)
	defer cleanup()
	registry := game.NewContinuationRegistry(game.ContinuationRegistryConfig{})
	client := newDedicatedGameClient(server)
	h.gameRouter = game.NewRouter(game.RouterConfigFrom("libera", "!", config.GameConfig{Enabled: true, Command: "avenger", PMEnabled: true, PMRejectMode: "help", MaxInputBytes: 512}), game.RouterDependencies{
		Users: users, PMState: db,
		Identities:    game.NewIdentityResolver(users, game.NewCaseMappingStore(), nil),
		Continuations: registry,
		Game:          game.NewActionHandler(client, registry, game.ActionHandlerConfig{Limits: game.BoundaryLimits{MaxInputBytes: 512, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1}, ConfigRevision: 1, PolicyRevision: 1}),
	})
	responses, err := h.HandleMessage(context.Background(), "Alice", "untrusted@host", "", "!avenger start", true, nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	joined := strings.Join(responses, " ")
	if !strings.Contains(joined, "Support ID:") || strings.Contains(joined, "private state") {
		t.Fatalf("unsafe malformed response handling: %q", joined)
	}
	key, _ := game.ContinuationKeyFor("libera", game.SessionIdentity{Kind: game.IdentityUnregistered, Value: "alice"})
	if _, status := registry.Resolve(key, "attack", time.Now()); status != game.ContinuationUnknown {
		t.Fatalf("malformed response installed context: %v", status)
	}
}

func TestGameSendFailurePreservesCommittedResultAndPrivateTarget(t *testing.T) {
	var committed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/game/action" {
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		var request game.ActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		committed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(game.ActionResponse{RequestID: request.RequestID, Status: "success", ResultCategory: "started", StateRevision: 1, StateChanged: true, Deliveries: []game.Delivery{{Target: game.DeliveryPM, Lines: []string{"<BOLD>Committed</BOLD>"}}}})
	}))
	defer server.Close()
	h, _, users, db, cleanup := newPMBoundaryHandler(t, true, false)
	defer cleanup()
	registry := game.NewContinuationRegistry(game.ContinuationRegistryConfig{})
	client := newDedicatedGameClient(server)
	h.gameRouter = game.NewRouter(game.RouterConfigFrom("libera", "!", config.GameConfig{Enabled: true, Command: "avenger", PMEnabled: true, PMRejectMode: "help", MaxInputBytes: 512}), game.RouterDependencies{
		Users: users, PMState: db,
		Identities:    game.NewIdentityResolver(users, game.NewCaseMappingStore(), nil),
		Continuations: registry,
		Game:          game.NewActionHandler(client, registry, game.ActionHandlerConfig{Limits: game.BoundaryLimits{MaxInputBytes: 512, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1}, ConfigRevision: 1, PolicyRevision: 1}),
	})
	var targets []string
	h.SetSendMessageFunc(func(target, message string) error {
		targets = append(targets, target)
		if !strings.Contains(message, "\x02Committed\x02") {
			t.Fatalf("formatter was bypassed: %q", message)
		}
		return errors.New("controlled queue/send failure")
	})
	responses, err := h.HandleMessage(context.Background(), "Alice", "untrusted@host", "", "!avenger start", true, nil)
	if err == nil || responses != nil {
		t.Fatalf("send failure not surfaced: responses=%v err=%v", responses, err)
	}
	if committed.Load() != 1 {
		t.Fatalf("delivery failure retried or rolled back committed action: %d", committed.Load())
	}
	if len(targets) == 0 {
		t.Fatal("final-target callback was not invoked")
	}
	for _, target := range targets {
		if target != "Alice" {
			t.Fatalf("private delivery targeted %q", target)
		}
	}
}
