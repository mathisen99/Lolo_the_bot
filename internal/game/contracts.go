// Package game defines the isolated, bounded Go/Python game boundary. It does
// not depend on generic command, mention, or tool contracts.
package game

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput      = errors.New("invalid_input")
	ErrRequestIDMismatch = errors.New("request_id_mismatch")
	ErrStaleRevision     = errors.New("stale_revision")
	ErrStaleContext      = errors.New("stale_context")
	ErrIdentityAmbiguous = errors.New("identity_ambiguous")
	ErrGameUnavailable   = errors.New("game_unavailable")
	ErrMigrationFailed   = errors.New("migration_failed")
	ErrResponseInvalid   = errors.New("response_invalid")
)

const (
	MaxIdentityBytes = 128
	MaxNickBytes     = 64
	MaxNetworkBytes  = 32
	MaxContinuations = 12
	MaxLineBytes     = 800
)

var (
	networkPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,63}$`)
	tokenPattern   = regexp.MustCompile(`^(?:[a-z][a-z0-9_-]{0,31}|[crm]-[a-z2-7]{6,26})$`)
)

type IdentityKind string

const (
	IdentityRegistered   IdentityKind = "registered_user"
	IdentityUnregistered IdentityKind = "unregistered_nick"
)

type SessionIdentity struct {
	Kind  IdentityKind `json:"kind"`
	Value string       `json:"value"`
}

func (i SessionIdentity) Validate() error {
	if i.Kind != IdentityRegistered && i.Kind != IdentityUnregistered {
		return fieldError("identity.kind", "unsupported identity kind")
	}
	if len(i.Value) < 1 || len([]byte(i.Value)) > MaxIdentityBytes || strings.ContainsAny(i.Value, "\x00\r\n") {
		return fieldError("identity.value", "must be 1-128 bytes without controls")
	}
	return nil
}

type SourceKind string

const (
	SourcePM      SourceKind = "pm"
	SourceChannel SourceKind = "channel"
)

type RequestSource struct {
	Kind            SourceKind `json:"kind"`
	Channel         string     `json:"channel"`
	EffectivePrefix string     `json:"effective_prefix"`
}

type Operation string

const (
	OperationAction           Operation = "action"
	OperationTransferIdentity Operation = "transfer_identity"
	OperationInvalidate       Operation = "invalidate_context"
)

type ActionMode string

const (
	ModeDirect      ActionMode = "direct"
	ModeAIInterpret ActionMode = "ai_interpret"
)

type ActionArguments struct {
	TargetID      string `json:"target_id,omitempty"`
	ItemID        string `json:"item_id,omitempty"`
	DestinationID string `json:"destination_id,omitempty"`
	Quantity      int    `json:"quantity,omitempty"`
	Page          int    `json:"page,omitempty"`
	Token         string `json:"token,omitempty"`
	Text          string `json:"text,omitempty"`
	Fallback      bool   `json:"fallback,omitempty"`
}

type Action struct {
	Name          string          `json:"name"`
	Arguments     ActionArguments `json:"arguments"`
	MenuContextID string          `json:"menu_context_id,omitempty"`
	ChoiceToken   string          `json:"choice_token,omitempty"`
}

type ClientContext struct {
	ContentPolicyRevision int64 `json:"content_policy_revision"`
	ConfigurationRevision int64 `json:"configuration_revision"`
}

type ActionRequest struct {
	RequestID             string          `json:"request_id"`
	IdempotencyKey        string          `json:"idempotency_key"`
	NetworkID             string          `json:"network_id"`
	Identity              SessionIdentity `json:"identity"`
	DisplayNick           string          `json:"display_nick"`
	Source                RequestSource   `json:"source"`
	Operation             Operation       `json:"operation"`
	Mode                  ActionMode      `json:"mode"`
	ExpectedStateRevision int64           `json:"expected_state_revision"`
	Action                Action          `json:"action"`
	ClientContext         ClientContext   `json:"client_context"`
}

type BoundaryLimits struct {
	MaxInputBytes        int
	MaxMenuLines         int
	MaxChoicesPerPage    int
	MaxNarrationBytes    int
	ActionTimeoutSeconds int
}

func (l BoundaryLimits) Reconcile(other BoundaryLimits) BoundaryLimits {
	return BoundaryLimits{minPositive(l.MaxInputBytes, other.MaxInputBytes), minPositive(l.MaxMenuLines, other.MaxMenuLines), minPositive(l.MaxChoicesPerPage, other.MaxChoicesPerPage), minPositive(l.MaxNarrationBytes, other.MaxNarrationBytes), minPositive(l.ActionTimeoutSeconds, other.ActionTimeoutSeconds)}
}
func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func (r ActionRequest) Validate(headerRequestID string, limits BoundaryLimits) error {
	if _, err := uuid.Parse(r.RequestID); err != nil {
		return fieldError("request_id", "must be a UUID")
	}
	if headerRequestID != r.RequestID {
		return fmt.Errorf("%w: X-Request-ID must equal request_id", ErrRequestIDMismatch)
	}
	if _, err := uuid.Parse(r.IdempotencyKey); err != nil {
		return fieldError("idempotency_key", "must be a UUID")
	}
	if !networkPattern.MatchString(r.NetworkID) {
		return fieldError("network_id", "invalid network identifier")
	}
	if err := r.Identity.Validate(); err != nil {
		return err
	}
	if len(r.DisplayNick) < 1 || len([]byte(r.DisplayNick)) > MaxNickBytes || strings.ContainsAny(r.DisplayNick, "\x00\r\n") {
		return fieldError("display_nick", "must be 1-64 bytes without controls")
	}
	if r.Source.Kind != SourcePM && r.Source.Kind != SourceChannel {
		return fieldError("source.kind", "unsupported source")
	}
	if r.Source.Kind == SourcePM && r.Source.Channel != "" {
		return fieldError("source.channel", "must be empty for PM")
	}
	if r.Source.Kind == SourceChannel && (len(r.Source.Channel) < 2 || r.Source.Channel[0] != '#' || len([]byte(r.Source.Channel)) > 64) {
		return fieldError("source.channel", "invalid IRC channel")
	}
	if len(r.Source.EffectivePrefix) < 1 || len([]byte(r.Source.EffectivePrefix)) > 8 {
		return fieldError("source.effective_prefix", "must be 1-8 bytes")
	}
	if r.Operation != OperationAction {
		return fieldError("operation", "action endpoint accepts only action")
	}
	if r.Mode != ModeDirect && r.Mode != ModeAIInterpret {
		return fieldError("mode", "unsupported action mode")
	}
	if r.ExpectedStateRevision < 0 {
		return fieldError("expected_state_revision", "must be non-negative")
	}
	if r.ClientContext.ConfigurationRevision < 1 || r.ClientContext.ContentPolicyRevision < 1 {
		return fieldError("client_context", "revisions must be positive")
	}
	return r.Action.Validate(r.Mode, limits.MaxInputBytes)
}

func (a Action) Validate(mode ActionMode, maxInputBytes int) error {
	allowed := map[string]bool{"resume": true, "start": true, "status": true, "inventory": true, "help": true, "credits": true, "reset": true, "quit": true, "content": true, "privacy": true, "delete": true, "look": true, "travel": true, "attack": true, "defend": true, "use": true, "equip": true, "buy": true, "sell": true, "escape": true, "recover": true, "investigate": true, "advance": true, "finalize": true, "next": true, "prev": true, "page": true, "ask": true}
	if !allowed[a.Name] {
		return fieldError("action.name", "unsupported action")
	}
	if a.MenuContextID != "" && !idPattern.MatchString(a.MenuContextID) {
		return fieldError("action.menu_context_id", "invalid context id")
	}
	if a.ChoiceToken != "" && !tokenPattern.MatchString(a.ChoiceToken) {
		return fieldError("action.choice_token", "invalid token")
	}
	args := a.Arguments
	if args.Quantity < 0 || args.Quantity > 99 {
		return fieldError("action.arguments.quantity", "must be between 0 and 99")
	}
	if args.Page < 0 || args.Page > 1000 {
		return fieldError("action.arguments.page", "must be between 0 and 1000")
	}
	for name, value := range map[string]string{"target_id": args.TargetID, "item_id": args.ItemID, "destination_id": args.DestinationID} {
		if value != "" && !idPattern.MatchString(value) {
			return fieldError("action.arguments."+name, "invalid identifier")
		}
	}
	if args.Token != "" && !tokenPattern.MatchString(args.Token) {
		return fieldError("action.arguments.token", "invalid token")
	}
	if args.Fallback && a.Name != "help" {
		return fieldError("action.arguments.fallback", "authored fallback is valid only for help")
	}
	if mode == ModeAIInterpret {
		if a.Name != "ask" || len(args.Text) < 1 || len([]byte(args.Text)) > maxInputBytes {
			return fieldError("action.arguments.text", "AI mode requires bounded ask text")
		}
	} else if args.Text != "" || a.Name == "ask" {
		return fieldError("action.arguments.text", "text is accepted only in AI interpret mode")
	}
	if err := validateActionSpecific(a.Name, args); err != nil {
		return err
	}
	return nil
}

func validateActionSpecific(name string, a ActionArguments) error {
	nonzero := func(v string) bool { return v != "" }
	switch name {
	case "travel":
		if !nonzero(a.DestinationID) {
			return fieldError("action.arguments.destination_id", "required for travel")
		}
	case "attack":
		if !nonzero(a.TargetID) {
			return fieldError("action.arguments.target_id", "required for attack")
		}
	case "use", "equip":
		if !nonzero(a.ItemID) {
			return fieldError("action.arguments.item_id", "required")
		}
	case "buy", "sell":
		if !nonzero(a.ItemID) || a.Quantity < 1 {
			return fieldError("action.arguments", "item_id and positive quantity required")
		}
	case "page":
		if a.Page < 1 {
			return fieldError("action.arguments.page", "positive page required")
		}
	case "reset":
		if a.Token != "" && !strings.HasPrefix(a.Token, "r-") {
			return fieldError("action.arguments.token", "reset token must use r- prefix")
		}
	}
	return nil
}

type DeliveryTarget string

const (
	DeliveryPM      DeliveryTarget = "pm"
	DeliveryChannel DeliveryTarget = "channel"
)

type Delivery struct {
	Target DeliveryTarget `json:"target"`
	Lines  []string       `json:"lines"`
}
type MenuContext struct {
	ID            string    `json:"id"`
	StateRevision int64     `json:"state_revision"`
	Page          int       `json:"page"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type Continuation struct {
	Input         string          `json:"input"`
	Kind          string          `json:"kind"`
	Action        string          `json:"action"`
	Arguments     ActionArguments `json:"arguments"`
	ChoiceToken   string          `json:"choice_token"`
	MenuContextID string          `json:"menu_context_id"`
	StateRevision int64           `json:"state_revision"`
}
type StableError struct {
	Category  string `json:"category"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
type ActionResponse struct {
	RequestID      string         `json:"request_id"`
	Status         string         `json:"status"`
	ResultCategory string         `json:"result_category"`
	StateRevision  int64          `json:"state_revision"`
	StateChanged   bool           `json:"state_changed"`
	Deliveries     []Delivery     `json:"deliveries"`
	MenuContext    *MenuContext   `json:"menu_context"`
	Continuations  []Continuation `json:"continuations"`
	Milestones     []string       `json:"milestones"`
	Error          *StableError   `json:"error"`
}

func (r ActionResponse) Validate(requestID string, limits BoundaryLimits) error {
	if r.RequestID != requestID {
		return fmt.Errorf("%w: response request_id differs", ErrRequestIDMismatch)
	}
	if r.Status != "success" && r.Status != "error" && r.Status != "unavailable" {
		return fieldError("status", "unsupported status")
	}
	if (r.Status == "success") != (r.Error == nil) {
		return fieldError("error", "success must omit error and failures must include one")
	}
	if r.ResultCategory == "" || len([]byte(r.ResultCategory)) > 64 || !idPattern.MatchString(r.ResultCategory) {
		return fieldError("result_category", "invalid result category")
	}
	if r.StateRevision < 0 {
		return fieldError("state_revision", "must be non-negative")
	}
	if len(r.Deliveries) > 2 {
		return fieldError("deliveries", "at most two deliveries allowed")
	}
	for _, d := range r.Deliveries {
		if d.Target != DeliveryPM && d.Target != DeliveryChannel {
			return fieldError("deliveries.target", "unsupported target")
		}
		if len(d.Lines) > limits.MaxMenuLines {
			return fieldError("deliveries.lines", "too many lines")
		}
		for _, line := range d.Lines {
			if len([]byte(line)) > MaxLineBytes || strings.ContainsAny(line, "\x00\r\n") {
				return fieldError("deliveries.lines", "line is oversized or unsafe")
			}
		}
	}
	if len(r.Continuations) > MaxContinuations || len(r.Continuations) > limits.MaxChoicesPerPage*2 {
		return fieldError("continuations", "too many continuations")
	}
	if len(r.Continuations) > 0 && r.MenuContext == nil {
		return fieldError("menu_context", "continuations require a menu context")
	}
	seen := map[string]bool{}
	for _, c := range r.Continuations {
		if !tokenPattern.MatchString(c.Input) || len([]byte(c.Input)) > 32 || seen[c.Input] {
			return fieldError("continuations.input", "invalid or duplicate continuation")
		}
		seen[c.Input] = true
		if c.StateRevision != r.StateRevision || c.MenuContextID == "" || r.MenuContext == nil || c.MenuContextID != r.MenuContext.ID {
			return fieldError("continuations", "revision/context mismatch")
		}
		binding := ContinuationBinding{Input: c.Input, Kind: c.Kind, Action: c.Action, Arguments: c.Arguments, ChoiceToken: c.ChoiceToken, MenuContextID: c.MenuContextID, StateRevision: c.StateRevision}
		if _, err := validateContinuationBinding(c.MenuContextID, binding); err != nil {
			return err
		}
	}
	if r.MenuContext != nil && (r.MenuContext.StateRevision != r.StateRevision || r.MenuContext.Page < 1 || r.MenuContext.ExpiresAt.IsZero()) {
		return fieldError("menu_context", "invalid revision, page, or expiry")
	}
	return nil
}

// ValidateForRequest applies response coherence and source-target/revision
// policy before any line is formatted, continuation is replaced, or delivery
// is attempted.
func (r ActionResponse) ValidateForRequest(request ActionRequest, limits BoundaryLimits) error {
	if err := r.Validate(request.RequestID, limits); err != nil {
		return err
	}
	if request.Source.Kind == SourcePM {
		for _, delivery := range r.Deliveries {
			if delivery.Target != DeliveryPM {
				return fieldError("deliveries.target", "PM actions may deliver only to PM")
			}
		}
	}
	if r.Status == "success" && r.StateChanged && r.StateRevision != request.ExpectedStateRevision+1 {
		return fieldError("state_revision", "state-changing success must increment exactly once")
	}
	if r.Status == "success" && !r.StateChanged && r.StateRevision < request.ExpectedStateRevision {
		return fieldError("state_revision", "read-only response cannot move revision backward")
	}
	return nil
}

type LifecycleRequest struct {
	RequestID             string           `json:"request_id"`
	NetworkID             string           `json:"network_id"`
	Operation             Operation        `json:"operation"`
	Identity              SessionIdentity  `json:"identity"`
	NewIdentity           *SessionIdentity `json:"new_identity,omitempty"`
	ConfigurationRevision int64            `json:"configuration_revision"`
}

func (r LifecycleRequest) Validate(header string) error {
	if _, e := uuid.Parse(r.RequestID); e != nil {
		return fieldError("request_id", "must be UUID")
	}
	if header != r.RequestID {
		return ErrRequestIDMismatch
	}
	if !networkPattern.MatchString(r.NetworkID) {
		return fieldError("network_id", "invalid")
	}
	if e := r.Identity.Validate(); e != nil {
		return e
	}
	if r.Operation != OperationTransferIdentity && r.Operation != OperationInvalidate {
		return fieldError("operation", "unsupported lifecycle operation")
	}
	if r.Operation == OperationTransferIdentity && (r.NewIdentity == nil || r.NewIdentity.Validate() != nil) {
		return fieldError("new_identity", "valid identity required")
	}
	if r.ConfigurationRevision < 1 {
		return fieldError("configuration_revision", "must be positive")
	}
	return nil
}

type LifecycleResponse struct {
	RequestID string       `json:"request_id"`
	Status    string       `json:"status"`
	Error     *StableError `json:"error"`
}

func (r LifecycleResponse) Validate(requestID string) error {
	if r.RequestID != requestID {
		return fmt.Errorf("%w: lifecycle response request_id differs", ErrRequestIDMismatch)
	}
	if r.Status != "success" && r.Status != "error" && r.Status != "unavailable" {
		return fieldError("status", "unsupported lifecycle status")
	}
	if (r.Status == "success") != (r.Error == nil) {
		return fieldError("error", "success must omit error and failures must include one")
	}
	return nil
}

type HealthStatus struct {
	Status            string `json:"status"`
	DatabaseAvailable bool   `json:"database_available"`
	SchemaVersion     int    `json:"schema_version"`
	MigrationStatus   string `json:"migration_status"`
	EngineVersion     string `json:"engine_version"`
	ContentVersion    string `json:"content_version"`
	ConfigRevision    int64  `json:"config_revision"`
	AIStatus          string `json:"ai_status"`
	ErrorCategory     string `json:"error_category,omitempty"`
}

func (h HealthStatus) Validate() error {
	if h.Status != "ready" && h.Status != "disabled" && h.Status != "degraded" {
		return fieldError("status", "unsupported health status")
	}
	if h.SchemaVersion < 0 || h.ConfigRevision < 1 {
		return fieldError("health", "invalid schema or configuration revision")
	}
	if h.MigrationStatus != "not_started" && h.MigrationStatus != "not_required" && h.MigrationStatus != "current" && h.MigrationStatus != "failed" {
		return fieldError("migration_status", "unsupported migration status")
	}
	if h.AIStatus != "disabled" && h.AIStatus != "disabled_missing_credentials" && h.AIStatus != "available" && h.AIStatus != "unavailable" {
		return fieldError("ai_status", "unsupported AI status")
	}
	if len([]byte(h.EngineVersion)) > 32 || len([]byte(h.ContentVersion)) > 64 {
		return fieldError("health", "version field is oversized")
	}
	return nil
}

func fieldError(field, message string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, message)
}
