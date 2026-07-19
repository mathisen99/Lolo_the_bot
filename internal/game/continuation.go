package game

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrContinuationCapacity = errors.New("continuation_capacity_exhausted")

// ContinuationKey is the complete canonical registry key. Network scope is
// never inferred from a nickname or shared across runtimes.
type ContinuationKey struct {
	NetworkID     string
	IdentityKind  IdentityKind
	IdentityValue string
}

func (k ContinuationKey) Validate() error {
	if !networkPattern.MatchString(k.NetworkID) {
		return fieldError("continuation.network_id", "invalid network identifier")
	}
	return (SessionIdentity{Kind: k.IdentityKind, Value: k.IdentityValue}).Validate()
}

// ContinuationBinding is routing-safe response data. Raw PM text and hostmask
// are never retained.
type ContinuationBinding struct {
	Input         string
	Kind          string
	Action        string
	Arguments     ActionArguments
	MenuContextID string
	ChoiceToken   string
	StateRevision int64
}

type continuationContext struct {
	id         string
	revision   int64
	expiresAt  time.Time
	lastAccess time.Time
	bindings   map[string]ContinuationBinding
	nickFold   string
}

type ContinuationRegistryConfig struct {
	Capacity    int
	MaxBindings int
	MaxTTL      time.Duration
	Now         func() time.Time
}

// ContinuationRegistry stores one current context per canonical identity.
type ContinuationRegistry struct {
	mu          sync.Mutex
	capacity    int
	maxBindings int
	maxTTL      time.Duration
	now         func() time.Time
	contexts    map[ContinuationKey]*continuationContext
	stale       map[ContinuationKey]map[string]struct{}
}

func NewContinuationRegistry(cfg ContinuationRegistryConfig) *ContinuationRegistry {
	if cfg.Capacity <= 0 {
		cfg.Capacity = 10000
	}
	if cfg.MaxBindings <= 0 || cfg.MaxBindings > MaxContinuations {
		cfg.MaxBindings = MaxContinuations
	}
	if cfg.MaxTTL <= 0 {
		cfg.MaxTTL = 15 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ContinuationRegistry{
		capacity: cfg.Capacity, maxBindings: cfg.MaxBindings, maxTTL: cfg.MaxTTL,
		now: cfg.Now, contexts: make(map[ContinuationKey]*continuationContext),
		stale: make(map[ContinuationKey]map[string]struct{}),
	}
}

func (r *ContinuationRegistry) Replace(key ContinuationKey, contextID string, bindings []ContinuationBinding, expiresAt time.Time) error {
	return r.replaceForNick(key, "", contextID, bindings, expiresAt)
}

func (r *ContinuationRegistry) ReplaceForNick(key ContinuationKey, nickFold, contextID string, bindings []ContinuationBinding, expiresAt time.Time) error {
	return r.replaceForNick(key, nickFold, contextID, bindings, expiresAt)
}

func (r *ContinuationRegistry) replaceForNick(key ContinuationKey, nickFold, contextID string, bindings []ContinuationBinding, expiresAt time.Time) error {
	if err := key.Validate(); err != nil {
		return err
	}
	now := r.now()
	if !idPattern.MatchString(contextID) {
		return fieldError("continuation.context_id", "invalid context id")
	}
	if expiresAt.IsZero() || !expiresAt.After(now) || expiresAt.After(now.Add(r.maxTTL)) {
		return fieldError("continuation.expires_at", "must be future and within configured TTL")
	}
	if len(bindings) == 0 || len(bindings) > r.maxBindings {
		return fieldError("continuation.bindings", "invalid binding count")
	}
	validated := make(map[string]ContinuationBinding, len(bindings))
	var revision int64 = -1
	for _, binding := range bindings {
		canonical, err := validateContinuationBinding(contextID, binding)
		if err != nil {
			return err
		}
		if _, duplicate := validated[canonical]; duplicate {
			return fieldError("continuation.input", "duplicate canonical input")
		}
		if revision == -1 {
			revision = binding.StateRevision
		} else if binding.StateRevision != revision {
			return fieldError("continuation.state_revision", "bindings must share a revision")
		}
		validated[canonical] = binding
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeExpiredLocked(now)
	previous, replacing := r.contexts[key]
	if !replacing && len(r.contexts) >= r.capacity {
		return ErrContinuationCapacity
	}
	if nickFold == "" && previous != nil {
		nickFold = previous.nickFold
	}
	if previous != nil {
		old := make(map[string]struct{}, len(previous.bindings))
		for input := range previous.bindings {
			old[input] = struct{}{}
		}
		r.stale[key] = old
	} else {
		delete(r.stale, key)
	}
	r.contexts[key] = &continuationContext{
		id: contextID, revision: revision, expiresAt: expiresAt, lastAccess: now,
		bindings: validated, nickFold: nickFold,
	}
	return nil
}

func validateContinuationBinding(contextID string, binding ContinuationBinding) (string, error) {
	if binding.StateRevision < 0 || binding.MenuContextID != contextID {
		return "", fieldError("continuation", "context or revision mismatch")
	}
	if strings.TrimSpace(binding.Input) != binding.Input || binding.Input == "" || len([]byte(binding.Input)) > 32 {
		return "", fieldError("continuation.input", "must be a bounded single token")
	}
	if !supportedContinuationAction(binding.Action) {
		return "", fieldError("continuation.action", "unsupported action")
	}
	action := Action{Name: binding.Action, Arguments: binding.Arguments, MenuContextID: binding.MenuContextID, ChoiceToken: binding.ChoiceToken}
	if err := action.Validate(ModeDirect, 0); err != nil {
		return "", err
	}

	switch binding.Kind {
	case "action":
		if !isLowerASCIIToken(binding.Input) || binding.Input != binding.Action || isBareNumber(binding.Input) {
			return "", fieldError("continuation.input", "action input must be its exact lowercase action")
		}
		return binding.Input, nil
	case "pagination":
		if !isLowerASCIIToken(binding.Input) || (binding.Input != "next" && binding.Input != "prev") || binding.Action != "page" || binding.Arguments.Page < 1 {
			return "", fieldError("continuation.input", "unsupported pagination input")
		}
		return binding.Input, nil
	case "choice":
		if !strings.HasPrefix(binding.Input, "c-") || !tokenPattern.MatchString(binding.Input) || binding.ChoiceToken != binding.Input {
			return "", fieldError("continuation.input", "invalid choice token")
		}
		return binding.Input, nil
	case "confirmation":
		if !strings.HasPrefix(binding.Input, "r-") || !tokenPattern.MatchString(binding.Input) || binding.Arguments.Token != binding.Input {
			return "", fieldError("continuation.input", "invalid confirmation token")
		}
		return binding.Input, nil
	default:
		return "", fieldError("continuation.kind", "unsupported continuation kind")
	}
}

func supportedContinuationAction(action string) bool {
	switch action {
	case "resume", "start", "status", "inventory", "help", "credits", "reset", "quit", "content", "privacy", "delete", "look", "travel", "attack", "defend", "use", "equip", "buy", "sell", "escape", "recover", "investigate", "advance", "finalize", "next", "prev", "page":
		return true
	default:
		return false
	}
}

func isLowerASCIIToken(value string) bool {
	if value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' && b != '-' {
			return false
		}
	}
	return true
}

func isBareNumber(value string) bool {
	if value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func canonicalContinuationInput(raw string) (string, bool) {
	trimmed := trimASCIIWhitespace(raw)
	if trimmed == "" || len([]byte(trimmed)) > 32 || isBareNumber(trimmed) {
		return "", false
	}
	if tokenPattern.MatchString(trimmed) && (strings.HasPrefix(trimmed, "c-") || strings.HasPrefix(trimmed, "r-")) {
		return trimmed, true
	}
	for _, b := range []byte(trimmed) {
		if b >= 'A' && b <= 'Z' {
			continue
		}
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' && b != '-' {
			return "", false
		}
	}
	return strings.ToLower(trimmed), true
}

func (r *ContinuationRegistry) Resolve(key ContinuationKey, raw string, now time.Time) (ContinuationBinding, ContinuationStatus) {
	canonical, ok := canonicalContinuationInput(raw)
	if !ok || key.Validate() != nil {
		return ContinuationBinding{}, ContinuationUnknown
	}
	if now.IsZero() {
		now = r.now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	context, exists := r.contexts[key]
	if !exists {
		if _, stale := r.stale[key][canonical]; stale {
			return ContinuationBinding{}, ContinuationStale
		}
		return ContinuationBinding{}, ContinuationUnknown
	}
	if !now.Before(context.expiresAt) {
		delete(r.contexts, key)
		delete(r.stale, key)
		return ContinuationBinding{}, ContinuationExpired
	}
	binding, exists := context.bindings[canonical]
	if !exists {
		if _, stale := r.stale[key][canonical]; stale {
			return ContinuationBinding{}, ContinuationStale
		}
		return ContinuationBinding{}, ContinuationUnknown
	}
	// Opaque tokens are exact lowercase ASCII. Named action inputs are the only
	// class for which ASCII case folding is allowed.
	if (binding.Kind == "choice" || binding.Kind == "confirmation") && trimASCIIWhitespace(raw) != canonical {
		return ContinuationBinding{}, ContinuationUnknown
	}
	context.lastAccess = now
	return binding, ContinuationCurrent
}

func (r *ContinuationRegistry) ResolvePMContinuation(key ContinuationKey, input string, now time.Time) (ContinuationBinding, ContinuationStatus) {
	return r.Resolve(key, input, now)
}

// Current returns the revision of the identity's unexpired current menu without
// resolving or exposing any choice token. Prefixed stable commands use this to
// carry the optimistic revision established by the latest validated response.
func (r *ContinuationRegistry) Current(key ContinuationKey, now time.Time) (int64, bool) {
	if key.Validate() != nil {
		return 0, false
	}
	if now.IsZero() {
		now = r.now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	context, exists := r.contexts[key]
	if !exists {
		return 0, false
	}
	if !now.Before(context.expiresAt) {
		delete(r.contexts, key)
		delete(r.stale, key)
		return 0, false
	}
	return context.revision, true
}

func (r *ContinuationRegistry) Invalidate(key ContinuationKey, _ string) {
	r.mu.Lock()
	delete(r.contexts, key)
	delete(r.stale, key)
	r.mu.Unlock()
}

func (r *ContinuationRegistry) InvalidateNick(networkID, nickFold, reason string) {
	r.mu.Lock()
	for key, context := range r.contexts {
		if key.NetworkID == networkID && context.nickFold == nickFold {
			delete(r.contexts, key)
			delete(r.stale, key)
		}
	}
	// The unregistered key is derivable even when no display alias was stored.
	key := ContinuationKey{NetworkID: networkID, IdentityKind: IdentityUnregistered, IdentityValue: nickFold}
	delete(r.contexts, key)
	delete(r.stale, key)
	r.mu.Unlock()
	_ = reason
}

func (r *ContinuationRegistry) ClearNetwork(networkID, _ string) {
	r.mu.Lock()
	for key := range r.contexts {
		if key.NetworkID == networkID {
			delete(r.contexts, key)
		}
	}
	for key := range r.stale {
		if key.NetworkID == networkID {
			delete(r.stale, key)
		}
	}
	r.mu.Unlock()
}

// Transfer moves only an existing unregistered context. A later user taking a
// nickname cannot trigger a transfer because absence of the old live context
// is a no-op. Destination conflict invalidates both contexts.
func (r *ContinuationRegistry) Transfer(networkID, oldCasefold, newCasefold string) error {
	oldKey := ContinuationKey{NetworkID: networkID, IdentityKind: IdentityUnregistered, IdentityValue: oldCasefold}
	newKey := ContinuationKey{NetworkID: networkID, IdentityKind: IdentityUnregistered, IdentityValue: newCasefold}
	if oldKey.Validate() != nil || newKey.Validate() != nil {
		return ErrIdentityAmbiguous
	}
	if oldKey == newKey {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	oldContext, exists := r.contexts[oldKey]
	if !exists {
		return nil
	}
	if _, conflict := r.contexts[newKey]; conflict {
		delete(r.contexts, oldKey)
		delete(r.contexts, newKey)
		delete(r.stale, oldKey)
		delete(r.stale, newKey)
		return ErrIdentityAmbiguous
	}
	delete(r.contexts, oldKey)
	oldContext.nickFold = newCasefold
	r.contexts[newKey] = oldContext
	if stale := r.stale[oldKey]; stale != nil {
		delete(r.stale, oldKey)
		r.stale[newKey] = stale
	}
	return nil
}

func (r *ContinuationRegistry) removeExpiredLocked(now time.Time) {
	for key, context := range r.contexts {
		if !now.Before(context.expiresAt) {
			delete(r.contexts, key)
			delete(r.stale, key)
		}
	}
}

func (k ContinuationKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.NetworkID, k.IdentityKind, k.IdentityValue)
}
