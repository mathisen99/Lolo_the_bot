package game

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestActionTelemetryPreservesRequestIDAndOmitsPrivateInput(t *testing.T) {
	var output bytes.Buffer
	telemetry := NewGameTelemetry(&output)
	var boundaryRequestID string
	handler := NewActionHandler(actionClientFunc(func(_ context.Context, request ActionRequest, _ time.Duration) (*ActionResponse, error) {
		boundaryRequestID = request.RequestID
		return successfulReadOnly(request), nil
	}), nil, ActionHandlerConfig{
		Limits:         BoundaryLimits{MaxInputBytes: 64, MaxMenuLines: 4, MaxChoicesPerPage: 6, ActionTimeoutSeconds: 1},
		ConfigRevision: 1, PolicyRevision: 1, Observer: telemetry,
	})
	input := testPMInput("status")
	input.Nick = "PrivateAlice"
	if _, err := handler.HandlePMGame(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	var event ActionEvent
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
		t.Fatalf("structured telemetry is invalid JSON: %v", err)
	}
	if event.RequestID == "" || event.RequestID != boundaryRequestID {
		t.Fatalf("request ID continuity lost: event=%q boundary=%q", event.RequestID, boundaryRequestID)
	}
	if event.SessionRef == "" || strings.Contains(event.SessionRef, "alice") {
		t.Fatalf("unsafe session reference: %q", event.SessionRef)
	}
	serialized := strings.ToLower(output.String())
	for _, forbidden := range []string{"privatealice", "hostmask", "password", "raw_prompt", "narration", "choice_token", "reset_token"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("telemetry contains forbidden value or field %q: %s", forbidden, serialized)
		}
	}
	if telemetry.MetricsSnapshot()["status|menu|"] != 1 {
		t.Fatalf("missing aggregate action metric: %#v", telemetry.MetricsSnapshot())
	}
}
