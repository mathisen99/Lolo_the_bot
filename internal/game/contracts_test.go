package game

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validRequest() ActionRequest {
	id := uuid.NewString()
	return ActionRequest{
		RequestID: id, IdempotencyKey: id, NetworkID: "libera",
		Identity: SessionIdentity{Kind: IdentityUnregistered, Value: "alice"}, DisplayNick: "Alice",
		Source: RequestSource{Kind: SourcePM, EffectivePrefix: "!"}, Operation: OperationAction,
		Mode: ModeDirect, ExpectedStateRevision: 0,
		Action:        Action{Name: "start", Arguments: ActionArguments{}},
		ClientContext: ClientContext{ContentPolicyRevision: 1, ConfigurationRevision: 1},
	}
}

func limits() BoundaryLimits {
	return BoundaryLimits{MaxInputBytes: 512, MaxMenuLines: 4, MaxChoicesPerPage: 6, MaxNarrationBytes: 600, ActionTimeoutSeconds: 10}
}

func TestActionRequestRequiresMatchingRequestIDs(t *testing.T) {
	r := validRequest()
	if err := r.Validate(uuid.NewString(), limits()); err == nil || !strings.Contains(err.Error(), "request_id_mismatch") {
		t.Fatalf("expected mismatch, got %v", err)
	}
	if err := r.Validate(r.RequestID, limits()); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestActionRequestRejectsMalformedOversizedAndWrongArguments(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ActionRequest)
	}{
		{"malformed identity", func(r *ActionRequest) { r.Identity.Value = "bad\nidentity" }},
		{"oversized nick", func(r *ActionRequest) { r.DisplayNick = strings.Repeat("x", 65) }},
		{"negative revision", func(r *ActionRequest) { r.ExpectedStateRevision = -1 }},
		{"missing attack target", func(r *ActionRequest) { r.Action.Name = "attack" }},
		{"direct text", func(r *ActionRequest) { r.Action.Arguments.Text = "not allowed" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validRequest()
			tc.mutate(&r)
			if err := r.Validate(r.RequestID, limits()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestAITextAndResponseBounds(t *testing.T) {
	r := validRequest()
	r.Mode = ModeAIInterpret
	r.Action.Name = "ask"
	r.Action.Arguments.Text = strings.Repeat("x", 513)
	if err := r.Validate(r.RequestID, limits()); err == nil {
		t.Fatal("expected oversized AI text rejection")
	}

	response := ActionResponse{RequestID: r.RequestID, Status: "success", ResultCategory: "menu", StateRevision: 0, Deliveries: []Delivery{{Target: DeliveryPM, Lines: []string{"ok"}}}, MenuContext: &MenuContext{ID: "m-context", StateRevision: 0, Page: 1, ExpiresAt: time.Now().Add(time.Minute)}}
	if err := response.Validate(r.RequestID, limits()); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	response.Deliveries[0].Lines = []string{"1", "2", "3", "4", "5"}
	if err := response.Validate(r.RequestID, limits()); err == nil {
		t.Fatal("expected line limit rejection")
	}
}

func TestBoundaryLimitsUseStricterPositiveValues(t *testing.T) {
	got := limits().Reconcile(BoundaryLimits{MaxInputBytes: 128, MaxMenuLines: 8, MaxChoicesPerPage: 3, MaxNarrationBytes: 700, ActionTimeoutSeconds: 4})
	if got.MaxInputBytes != 128 || got.MaxMenuLines != 4 || got.MaxChoicesPerPage != 3 || got.MaxNarrationBytes != 600 || got.ActionTimeoutSeconds != 4 {
		t.Fatalf("unexpected reconciliation: %#v", got)
	}
}
