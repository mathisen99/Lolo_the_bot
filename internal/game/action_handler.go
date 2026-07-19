package game

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/lolo/internal/config"
)

// ActionClient is the dedicated, non-streaming game HTTP boundary. Generic
// command, mention, history, and tool methods are intentionally absent.
type ActionClient interface {
	SendGameAction(context.Context, ActionRequest, time.Duration) (*ActionResponse, error)
}

// ActionHandlerConfig contains the stricter Go-side controls applied before an
// action may cross the HTTP boundary.
type ActionHandlerConfig struct {
	Limits            BoundaryLimits
	ConfigRevision    int64
	PolicyRevision    int64
	MaxPending        int
	DirectBurst       int
	DirectWindow      time.Duration
	DirectCooldown    time.Duration
	AIRequests        int
	AIBurst           int
	AIWindow          time.Duration
	AIEnabled         bool
	MenuContextMaxTTL time.Duration
	Observer          ActionObserver
	Now               func() time.Time
}

func ActionHandlerConfigFrom(cfg config.GameConfig) ActionHandlerConfig {
	return ActionHandlerConfig{
		Limits: BoundaryLimits{
			MaxInputBytes: cfg.MaxInputBytes, MaxMenuLines: cfg.MaxMenuLines,
			MaxChoicesPerPage: cfg.MaxChoicesPerPage, MaxNarrationBytes: cfg.MaxNarrationBytes,
			ActionTimeoutSeconds: cfg.ActionTimeoutSeconds,
		},
		ConfigRevision: cfg.ConfigRevision, PolicyRevision: cfg.ContentPolicyRevision,
		MaxPending:  cfg.MaxPendingActionsPerPlayer,
		DirectBurst: cfg.ActionBurst, DirectWindow: time.Duration(cfg.ActionWindowSeconds) * time.Second,
		DirectCooldown: time.Duration(cfg.ActionCooldownMS) * time.Millisecond,
		AIRequests:     cfg.RateLimits.AI.Requests, AIBurst: cfg.RateLimits.AI.Burst,
		AIWindow:          time.Duration(cfg.RateLimits.AI.WindowSeconds) * time.Second,
		AIEnabled:         cfg.AIEnhancementEnabled,
		MenuContextMaxTTL: time.Duration(cfg.MenuContextTTLSeconds) * time.Second,
	}
}

type actionBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	updatedAt  time.Time
	lastUse    time.Time
	cooldown   time.Duration
}

type admissionState struct {
	direct  actionBucket
	ai      actionBucket
	pending int
}

// ActionHandler turns already-authorized PM inputs into dedicated action
// requests. It owns game-only rate buckets and pending admission.
type ActionHandler struct {
	client   ActionClient
	registry *ContinuationRegistry
	cfg      ActionHandlerConfig
	now      func() time.Time
	observer ActionObserver

	mu     sync.Mutex
	states map[ContinuationKey]*admissionState
}

func NewActionHandler(client ActionClient, registry *ContinuationRegistry, cfg ActionHandlerConfig) *ActionHandler {
	if cfg.Limits.MaxInputBytes <= 0 {
		cfg.Limits.MaxInputBytes = 512
	}
	if cfg.Limits.MaxMenuLines <= 0 {
		cfg.Limits.MaxMenuLines = 4
	}
	if cfg.Limits.MaxChoicesPerPage <= 0 {
		cfg.Limits.MaxChoicesPerPage = 6
	}
	if cfg.Limits.ActionTimeoutSeconds <= 0 {
		cfg.Limits.ActionTimeoutSeconds = 10
	}
	if cfg.ConfigRevision < 1 {
		cfg.ConfigRevision = 1
	}
	if cfg.PolicyRevision < 1 {
		cfg.PolicyRevision = 1
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 2
	}
	if cfg.DirectBurst <= 0 {
		cfg.DirectBurst = 4
	}
	if cfg.DirectWindow <= 0 {
		cfg.DirectWindow = 10 * time.Second
	}
	if cfg.AIRequests <= 0 {
		cfg.AIRequests = 2
	}
	if cfg.AIBurst <= 0 {
		cfg.AIBurst = 1
	}
	if cfg.AIWindow <= 0 {
		cfg.AIWindow = 10 * time.Minute
	}
	if cfg.MenuContextMaxTTL <= 0 {
		cfg.MenuContextMaxTTL = 15 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ActionHandler{client: client, registry: registry, cfg: cfg, now: cfg.Now, observer: cfg.Observer, states: make(map[ContinuationKey]*admissionState)}
}

func (h *ActionHandler) HandlePMGame(ctx context.Context, input PMGameInput) ([]Delivery, error) {
	key, err := ContinuationKeyFor(input.NetworkID, input.Identity)
	if err != nil {
		return safeGameError("invalid game identity", ""), nil
	}
	action, mode, revision, err := h.parseInput(input, key)
	if err != nil {
		return []Delivery{{Target: DeliveryPM, Lines: []string{"Invalid game action. Use " + input.Prefix + "avenger help."}}}, nil
	}
	if mode == ModeAIInterpret && !h.cfg.AIEnabled {
		return []Delivery{{Target: DeliveryPM, Lines: []string{"AI interpretation is disabled; use the displayed direct game actions."}}}, nil
	}
	if !h.admit(key, mode, h.now()) {
		return []Delivery{{Target: DeliveryPM, Lines: []string{"Game action limit reached; please wait and try again."}}}, nil
	}
	defer h.release(key)

	started := h.now()
	requestID := uuid.NewString()
	request := ActionRequest{
		RequestID: requestID, IdempotencyKey: requestID, NetworkID: input.NetworkID,
		Identity: input.Identity, DisplayNick: input.Nick,
		Source:    RequestSource{Kind: SourcePM, EffectivePrefix: input.Prefix},
		Operation: OperationAction, Mode: mode, ExpectedStateRevision: revision,
		Action:        action,
		ClientContext: ClientContext{ContentPolicyRevision: h.cfg.PolicyRevision, ConfigurationRevision: h.cfg.ConfigRevision},
	}
	if err := request.Validate(requestID, h.cfg.Limits); err != nil {
		h.observe(request, started, revision, "invalid_request", "invalid_input")
		return safeGameError("invalid game request", requestID), nil
	}
	response, err := h.client.SendGameAction(ctx, request, time.Duration(h.cfg.Limits.ActionTimeoutSeconds)*time.Second)
	if err != nil {
		h.observe(request, started, revision, "game_unavailable", "game_unavailable")
		return safeGameError("game is temporarily unavailable", requestID), nil
	}
	if err := response.ValidateForRequest(request, h.cfg.Limits); err != nil {
		h.observe(request, started, revision, "response_invalid", "response_invalid")
		return safeGameError("game response could not be safely displayed", requestID), nil
	}

	// Install the replacement only after the complete response has validated.
	// A capacity failure disables bare continuation but never discards a
	// committed result or its safe deliveries.
	if response.MenuContext != nil && len(response.Continuations) > 0 && h.registry != nil {
		bindings := make([]ContinuationBinding, 0, len(response.Continuations))
		for _, continuation := range response.Continuations {
			bindings = append(bindings, ContinuationBinding{
				Input: continuation.Input, Kind: continuation.Kind, Action: continuation.Action,
				Arguments: continuation.Arguments, ChoiceToken: continuation.ChoiceToken,
				MenuContextID: continuation.MenuContextID, StateRevision: continuation.StateRevision,
			})
		}
		expiresAt := response.MenuContext.ExpiresAt
		if expiresAt.After(h.now().Add(h.cfg.MenuContextMaxTTL)) {
			h.observe(request, started, response.StateRevision, "response_invalid", "response_invalid")
			return safeGameError("game response could not be safely displayed", requestID), nil
		}
		_ = h.registry.ReplaceForNick(key, input.Identity.Value, response.MenuContext.ID, bindings, expiresAt)
	}
	errorCategory := ""
	if response.Error != nil {
		errorCategory = response.Error.Category
	}
	h.observe(request, started, response.StateRevision, response.ResultCategory, errorCategory)
	return append([]Delivery(nil), response.Deliveries...), nil
}

func (h *ActionHandler) observe(request ActionRequest, started time.Time, postRevision int64, resultCategory, errorCategory string) {
	if h.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	latency := h.now().Sub(started).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	resultCategory = safeTelemetryCategory(resultCategory, "unknown_result")
	if errorCategory != "" {
		errorCategory = safeTelemetryCategory(errorCategory, "unknown_error")
	}
	h.observer.ObserveGameAction(ActionEvent{
		RequestID: request.RequestID, Network: request.NetworkID,
		SessionRef: safeSessionReference(request.NetworkID, request.Identity),
		ActionType: request.Action.Name, PreRevision: request.ExpectedStateRevision,
		PostRevision: postRevision, LatencyMilliseconds: latency,
		ResultCategory: resultCategory, ErrorCategory: errorCategory,
		ConfigurationRevision: request.ClientContext.ConfigurationRevision,
		ContentPolicyRevision: request.ClientContext.ContentPolicyRevision,
	})
}

func (h *ActionHandler) parseInput(input PMGameInput, key ContinuationKey) (Action, ActionMode, int64, error) {
	if input.Continuation != nil {
		binding := input.Continuation
		return Action{Name: binding.Action, Arguments: binding.Arguments, MenuContextID: binding.MenuContextID, ChoiceToken: binding.ChoiceToken}, ModeDirect, binding.StateRevision, nil
	}
	if len([]byte(input.CommandText)) > h.cfg.Limits.MaxInputBytes {
		return Action{}, "", 0, ErrInvalidInput
	}
	text := trimASCIIWhitespace(input.CommandText)
	revision := int64(0)
	if h.registry != nil {
		if current, ok := h.registry.Current(key, h.now()); ok {
			revision = current
		}
		// Exact displayed action, choice, confirmation, and pagination tokens
		// remain valid when used inside the explicit avenger namespace.
		if binding, status := h.registry.Resolve(key, text, h.now()); status == ContinuationCurrent {
			return Action{Name: binding.Action, Arguments: binding.Arguments, MenuContextID: binding.MenuContextID, ChoiceToken: binding.ChoiceToken}, ModeDirect, binding.StateRevision, nil
		}
	}
	if text == "" {
		return Action{Name: "resume"}, ModeDirect, revision, nil
	}
	fallback := func() (Action, ActionMode, int64, error) {
		return Action{Name: "help", Arguments: ActionArguments{Fallback: true}}, ModeDirect, revision, nil
	}
	fields := strings.Fields(text)
	name := strings.ToLower(fields[0])
	action := Action{Name: name}
	mode := ModeDirect
	arg := func(index int) (string, error) {
		if len(fields) != index+1 {
			return "", ErrInvalidInput
		}
		return strings.ToLower(fields[index]), nil
	}
	var value string
	var err error
	switch name {
	case "travel":
		value, err = arg(1)
		action.Arguments.DestinationID = value
	case "attack":
		value, err = arg(1)
		action.Arguments.TargetID = value
	case "use", "equip":
		value, err = arg(1)
		action.Arguments.ItemID = value
	case "buy", "sell":
		if len(fields) != 3 {
			return fallback()
		}
		action.Arguments.ItemID = strings.ToLower(fields[1])
		action.Arguments.Quantity, err = strconv.Atoi(fields[2])
	case "page":
		value, err = arg(1)
		if err == nil {
			action.Arguments.Page, err = strconv.Atoi(value)
		}
	case "reset":
		if len(fields) > 2 {
			return fallback()
		}
		if len(fields) == 2 {
			action.Arguments.Token = fields[1]
		}
	case "content":
		if len(fields) > 2 || (len(fields) == 2 && strings.ToLower(fields[1]) != "status") {
			return fallback()
		}
		if len(fields) == 2 {
			action.Arguments.Token = "status"
		}
	case "ask":
		if len(fields) < 2 {
			return fallback()
		}
		mode = ModeAIInterpret
		action.Arguments.Text = strings.TrimSpace(text[len(fields[0]):])
	default:
		if len(fields) != 1 {
			return fallback()
		}
	}
	if err != nil || action.Validate(mode, h.cfg.Limits.MaxInputBytes) != nil {
		return fallback()
	}
	return action, mode, revision, nil
}

func (h *ActionHandler) admit(key ContinuationKey, mode ActionMode, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.states[key]
	if state == nil {
		state = &admissionState{
			direct: newActionBucket(h.cfg.DirectBurst, h.cfg.DirectBurst, h.cfg.DirectWindow, h.cfg.DirectCooldown, now),
			ai:     newActionBucket(h.cfg.AIRequests, h.cfg.AIBurst, h.cfg.AIWindow, 0, now),
		}
		h.states[key] = state
	}
	if state.pending >= h.cfg.MaxPending {
		return false
	}
	bucket := &state.direct
	if mode == ModeAIInterpret {
		bucket = &state.ai
	}
	if !takeActionToken(bucket, now) {
		return false
	}
	state.pending++
	return true
}

func newActionBucket(requests, burst int, window, cooldown time.Duration, now time.Time) actionBucket {
	if requests <= 0 {
		requests = 1
	}
	if burst <= 0 {
		burst = requests
	}
	return actionBucket{tokens: float64(burst), capacity: float64(burst), refillRate: float64(requests) / window.Seconds(), updatedAt: now, cooldown: cooldown}
}

func takeActionToken(bucket *actionBucket, now time.Time) bool {
	elapsed := now.Sub(bucket.updatedAt).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * bucket.refillRate
		if bucket.tokens > bucket.capacity {
			bucket.tokens = bucket.capacity
		}
		bucket.updatedAt = now
	}
	if !bucket.lastUse.IsZero() && now.Sub(bucket.lastUse) < bucket.cooldown {
		return false
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	bucket.lastUse = now
	return true
}

func (h *ActionHandler) release(key ContinuationKey) {
	h.mu.Lock()
	if state := h.states[key]; state != nil && state.pending > 0 {
		state.pending--
	}
	h.mu.Unlock()
}

func safeGameError(message, requestID string) []Delivery {
	line := strings.TrimSpace(message)
	if requestID != "" {
		line = fmt.Sprintf("%s. Support ID: %s", line, requestID)
	} else if !strings.HasSuffix(line, ".") {
		line += "."
	}
	return []Delivery{{Target: DeliveryPM, Lines: []string{line}}}
}

var _ PMGameHandler = (*ActionHandler)(nil)
