package game

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
)

// PMRouteKind identifies the one terminal route selected for a private message.
type PMRouteKind uint8

const (
	PMRouteSilent PMRouteKind = iota
	PMRouteVerify
	PMRouteGameCommand
	PMRouteContinuation
	PMRouteRejected
)

// RouteInput contains transport data used only at the Go routing boundary.
// Hostmask is available to the existing VerifyCommand but is never copied into
// a game request.
type RouteInput struct {
	NetworkID  string
	Nick       string
	Hostmask   string
	Channel    string
	Message    string
	IsPM       bool
	Prefix     string
	ReceivedAt time.Time
}

// RouteOutput is terminal when Handled is true. RoutePM always returns a
// terminal output, including on rejection and internal errors.
type RouteOutput struct {
	Kind       PMRouteKind
	Deliveries []Delivery
	Handled    bool
}

// PMGameInput is the narrow hand-off to the dedicated game path. It
// intentionally has no hostmask, password, generic command, mention, prompt,
// streaming, or tool fields.
type PMGameInput struct {
	NetworkID    string
	Identity     SessionIdentity
	Nick         string
	Prefix       string
	CommandText  string
	Continuation *ContinuationBinding
}

// PMGameHandler is implemented by the dedicated game action client. It must
// not dispatch through the generic Python command or mention endpoints.
type PMGameHandler interface {
	HandlePMGame(context.Context, PMGameInput) ([]Delivery, error)
}

// ContinuationStatus distinguishes a currently valid exact binding from all
// fail-closed classes. Task 3 supplies the stateful registry implementation.
type ContinuationStatus uint8

const (
	ContinuationUnknown ContinuationStatus = iota
	ContinuationCurrent
	ContinuationStale
	ContinuationExpired
	ContinuationAmbiguous
)

// ContinuationResolver performs exact current-context lookup. Implementations
// must not call Python, an LLM, generic dispatch, or tools.
type ContinuationResolver interface {
	ResolvePMContinuation(key ContinuationKey, input string, now time.Time) (ContinuationBinding, ContinuationStatus)
}

// PMStateReader preserves the existing database-backed PM enabled policy.
type PMStateReader interface {
	GetPMState() (bool, error)
}

// PMUserLookup is deliberately limited to a database lookup. It has no WHOIS
// method, making the ignored-user safety filter safe for unregistered users.
type PMUserLookup interface {
	GetUser(nick string) (*database.User, error)
}

// RouterConfig is an immutable snapshot of settings needed before any PM can
// reach an executable path.
type RouterConfig struct {
	NetworkID            string
	Prefix               string
	Command              string
	Enabled              bool
	PMEnabled            bool
	PMRejectMode         string
	ChannelHandoffNotice bool
	MaxInputBytes        int
	HelpInterval         time.Duration
}

// RouterConfigFrom converts the operator configuration into the narrow game
// routing snapshot.
func RouterConfigFrom(networkID, prefix string, cfg config.GameConfig) RouterConfig {
	return RouterConfig{
		NetworkID: networkID, Prefix: prefix, Command: cfg.Command,
		Enabled: cfg.Enabled, PMEnabled: cfg.PMEnabled,
		PMRejectMode: cfg.PMRejectMode, ChannelHandoffNotice: cfg.ChannelHandoffNotice,
		MaxInputBytes: cfg.MaxInputBytes,
	}
}

// RouterDependencies are explicit so tests can prove that only the selected
// terminal route is reachable.
type RouterDependencies struct {
	Users         PMUserLookup
	PMState       PMStateReader
	Verify        commands.Command
	Game          PMGameHandler
	Identities    *IdentityResolver
	Continuations ContinuationResolver
	Now           func() time.Time
}

// Router owns the exhaustive fail-closed PM allowlist.
type Router struct {
	cfg           RouterConfig
	users         PMUserLookup
	pmState       PMStateReader
	verify        commands.Command
	game          PMGameHandler
	identities    *IdentityResolver
	continuations ContinuationResolver
	now           func() time.Time

	helpMu   sync.Mutex
	lastHelp map[string]time.Time
}

func NewRouter(cfg RouterConfig, deps RouterDependencies) *Router {
	if cfg.Command == "" {
		cfg.Command = "avenger"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "!"
	}
	if cfg.MaxInputBytes <= 0 {
		cfg.MaxInputBytes = 512
	}
	if cfg.PMRejectMode != "silent" {
		cfg.PMRejectMode = "help"
	}
	if cfg.HelpInterval <= 0 {
		cfg.HelpInterval = 30 * time.Second
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Router{
		cfg: cfg, users: deps.Users, pmState: deps.PMState, verify: deps.Verify,
		game: deps.Game, identities: deps.Identities, continuations: deps.Continuations, now: now,
		lastHelp: make(map[string]time.Time),
	}
}

// Enabled reports the immutable master feature state for this network-local
// router. Callers use it to restore the pre-feature PM path during rollback.
func (r *Router) Enabled() bool {
	return r != nil && r.cfg.Enabled
}

// RoutePMFeatureOff isolates verification passwords and reserves the game
// namespace while allowing every other PM to follow the pre-feature handler.
// It never resolves game identity or calls Python, so rollback cannot issue a
// WHOIS or mutate retained game data.
func (r *Router) RoutePMFeatureOff(in RouteInput) (RouteOutput, error) {
	out := RouteOutput{Kind: PMRouteSilent, Handled: false}
	if !in.IsPM {
		return out, fmt.Errorf("RoutePMFeatureOff requires a private message")
	}
	if in.NetworkID == "" {
		in.NetworkID = r.cfg.NetworkID
	}
	if in.Prefix == "" {
		in.Prefix = r.cfg.Prefix
	}

	message := trimASCIIWhitespace(in.Message)
	argsText, verify := exactNamespace(message, in.Prefix, "verify")
	_, gameCommand := exactNamespace(message, in.Prefix, r.cfg.Command)
	if !verify && !gameCommand {
		return out, nil
	}

	ignored, err := r.isIgnored(in.Nick)
	if err != nil {
		return RouteOutput{Kind: PMRouteSilent, Handled: true}, err
	}
	if ignored {
		return RouteOutput{Kind: PMRouteSilent, Handled: true}, nil
	}
	if verify {
		return r.routeVerify(in, argsText)
	}
	return RouteOutput{
		Kind: PMRouteRejected, Handled: true,
		Deliveries: []Delivery{{Target: DeliveryPM, Lines: []string{"Game is unavailable."}}},
	}, nil
}

// RouteChannel reserves the exact game namespace before generic command
// dispatch. The private-solo release does not execute gameplay from channels;
// it either emits one concise PM handoff or consumes the command silently.
func (r *Router) RouteChannel(in RouteInput) (RouteOutput, error) {
	out := RouteOutput{Kind: PMRouteSilent, Handled: false}
	if in.IsPM {
		return out, fmt.Errorf("RouteChannel requires a channel message")
	}
	if in.NetworkID == "" {
		in.NetworkID = r.cfg.NetworkID
	}
	if in.Prefix == "" {
		in.Prefix = r.cfg.Prefix
	}
	if in.Channel == "" {
		return out, fmt.Errorf("RouteChannel requires a channel")
	}

	if _, ok := channelNamespace(in.Message, in.Prefix, r.cfg.Command); !ok {
		return out, nil
	}
	out.Handled = true

	ignored, err := r.isIgnored(in.Nick)
	if err != nil {
		return out, err
	}
	if ignored {
		return out, nil
	}
	if !r.cfg.Enabled {
		out.Kind = PMRouteRejected
		out.Deliveries = []Delivery{{Target: DeliveryChannel, Lines: []string{"Game is unavailable."}}}
		return out, nil
	}
	allowed, err := r.gamePMAllowed()
	if err != nil {
		return out, err
	}
	if !allowed {
		out.Kind = PMRouteRejected
		out.Deliveries = []Delivery{{Target: DeliveryChannel, Lines: []string{"Game is unavailable."}}}
		return out, nil
	}
	if len([]byte(in.Message)) > r.cfg.MaxInputBytes || !r.cfg.ChannelHandoffNotice {
		return out, nil
	}

	out.Kind = PMRouteRejected
	out.Deliveries = []Delivery{{
		Target: DeliveryChannel,
		Lines:  []string{fmt.Sprintf("Avenger gameplay is private. Message me with %s%s help.", r.cfg.Prefix, r.cfg.Command)},
	}}
	return out, nil
}

// RoutePM is the first executable PM branch while the feature is enabled. It
// always consumes the input so callers can never fall through to logging,
// generic dispatch, mentions, streaming, LLM chat, image handling, or tools.
func (r *Router) RoutePM(ctx context.Context, in RouteInput) (RouteOutput, error) {
	out := RouteOutput{Kind: PMRouteSilent, Handled: true}
	if !in.IsPM {
		return out, fmt.Errorf("RoutePM requires a private message")
	}
	if in.NetworkID == "" {
		in.NetworkID = r.cfg.NetworkID
	}
	if in.Prefix == "" {
		in.Prefix = r.cfg.Prefix
	}
	if in.ReceivedAt.IsZero() {
		in.ReceivedAt = r.now()
	}

	// Safety filters precede all parsing and executable routes.
	ignored, err := r.isIgnored(in.Nick)
	if err != nil {
		return out, err
	}
	if ignored {
		return out, nil
	}
	if len([]byte(in.Message)) > r.cfg.MaxInputBytes {
		return r.reject(in, in.ReceivedAt), nil
	}

	message := trimASCIIWhitespace(in.Message)
	if argsText, ok := exactNamespace(message, in.Prefix, "verify"); ok {
		return r.routeVerify(in, argsText)
	}
	if commandText, ok := exactNamespace(message, in.Prefix, r.cfg.Command); ok {
		return r.routeGame(ctx, in, commandText, nil, PMRouteGameCommand)
	}
	identity, key, ok := r.resolveIdentity(ctx, in)
	if !ok {
		return r.reject(in, in.ReceivedAt), nil
	}
	if r.continuations != nil {
		binding, status := r.continuations.ResolvePMContinuation(key, message, in.ReceivedAt)
		if status == ContinuationCurrent {
			return r.routeGameResolved(ctx, in, identity, key, "", &binding, PMRouteContinuation)
		}
	}
	return r.reject(in, in.ReceivedAt), nil
}

func (r *Router) routeVerify(in RouteInput, argsText string) (RouteOutput, error) {
	out := RouteOutput{Kind: PMRouteVerify, Handled: true}
	if r.verify == nil || !strings.EqualFold(r.verify.Name(), "verify") {
		return out, fmt.Errorf("existing VerifyCommand is unavailable")
	}
	args := strings.Fields(argsText)
	ctx := commands.NewContextForNetwork(
		"verify", args, in.Message, in.Nick, in.Hostmask, in.NetworkID,
		"", true, database.LevelNormal, false, in.Prefix,
	)
	response, err := r.verify.Execute(ctx)
	if err != nil {
		return out, err
	}
	if response != nil && response.Message != "" {
		out.Deliveries = []Delivery{{Target: DeliveryPM, Lines: []string{response.Message}}}
	}
	return out, nil
}

func (r *Router) routeGame(ctx context.Context, in RouteInput, commandText string, binding *ContinuationBinding, kind PMRouteKind) (RouteOutput, error) {
	identity, key, ok := r.resolveIdentity(ctx, in)
	if !ok {
		return r.reject(in, in.ReceivedAt), nil
	}
	return r.routeGameResolved(ctx, in, identity, key, commandText, binding, kind)
}

func (r *Router) routeGameResolved(ctx context.Context, in RouteInput, identity SessionIdentity, key ContinuationKey, commandText string, binding *ContinuationBinding, kind PMRouteKind) (RouteOutput, error) {
	out := RouteOutput{Kind: kind, Handled: true}
	allowed, err := r.gamePMAllowed()
	if err != nil {
		return r.reject(in, in.ReceivedAt), err
	}
	if !allowed {
		return r.reject(in, in.ReceivedAt), nil
	}
	if isQuitCommand(commandText, binding) {
		if invalidator, ok := r.continuations.(interface {
			Invalidate(ContinuationKey, string)
		}); ok {
			invalidator.Invalidate(key, "quit")
		}
	}
	if r.game == nil {
		out.Deliveries = []Delivery{{Target: DeliveryPM, Lines: []string{"Game is temporarily unavailable."}}}
		return out, nil
	}
	deliveries, err := r.game.HandlePMGame(ctx, PMGameInput{
		NetworkID: in.NetworkID, Identity: identity, Nick: in.Nick, Prefix: in.Prefix,
		CommandText: commandText, Continuation: binding,
	})
	if err != nil {
		return out, err
	}
	out.Deliveries = deliveries
	return out, nil
}

func (r *Router) resolveIdentity(ctx context.Context, in RouteInput) (SessionIdentity, ContinuationKey, bool) {
	if r.identities == nil {
		return SessionIdentity{}, ContinuationKey{}, false
	}
	identity, err := r.identities.Resolve(ctx, in.NetworkID, in.Nick, in.Hostmask)
	if err != nil {
		// Ambiguity is terminal. Remove any nickname context that could otherwise
		// expose a session through a later classification attempt.
		if invalidator, ok := r.continuations.(interface {
			InvalidateNick(string, string, string)
		}); ok {
			invalidator.InvalidateNick(in.NetworkID, r.identities.FoldNick(in.NetworkID, in.Nick), "identity_ambiguous")
		}
		return SessionIdentity{}, ContinuationKey{}, false
	}
	key, err := ContinuationKeyFor(in.NetworkID, identity)
	if err != nil {
		return SessionIdentity{}, ContinuationKey{}, false
	}
	return identity, key, true
}

func isQuitCommand(commandText string, binding *ContinuationBinding) bool {
	if binding != nil {
		return binding.Action == "quit"
	}
	fields := strings.Fields(commandText)
	return len(fields) > 0 && strings.EqualFold(fields[0], "quit")
}

func (r *Router) gamePMAllowed() (bool, error) {
	if !r.cfg.Enabled || !r.cfg.PMEnabled {
		return false, nil
	}
	if r.pmState == nil {
		return false, nil
	}
	return r.pmState.GetPMState()
}

func (r *Router) isIgnored(nick string) (bool, error) {
	if r.users == nil {
		return false, nil
	}
	usr, err := r.users.GetUser(nick)
	if err != nil {
		return false, err
	}
	return usr != nil && usr.Level == database.LevelIgnored, nil
}

func (r *Router) reject(in RouteInput, now time.Time) RouteOutput {
	out := RouteOutput{Kind: PMRouteRejected, Handled: true}
	if r.cfg.PMRejectMode == "silent" {
		return out
	}
	key := in.NetworkID + "\x00" + in.Nick
	r.helpMu.Lock()
	last := r.lastHelp[key]
	if !last.IsZero() && now.Sub(last) < r.cfg.HelpInterval {
		r.helpMu.Unlock()
		return out
	}
	r.lastHelp[key] = now
	r.helpMu.Unlock()
	line := fmt.Sprintf("Game PM syntax: %s%s help (or reply with a current game choice).", in.Prefix, r.cfg.Command)
	out.Deliveries = []Delivery{{Target: DeliveryPM, Lines: []string{line}}}
	return out
}

func channelNamespace(message, prefix, name string) (string, bool) {
	// Match Dispatcher.ParseCommand exactly for channel command boundaries:
	// the prefix must start at byte zero, while whitespace after it is Unicode-
	// aware. This prevents a command accepted by the dispatcher from bypassing
	// the reserved game route.
	if prefix == "" || !strings.HasPrefix(message, prefix) {
		return "", false
	}
	rest := strings.TrimSpace(message[len(prefix):])
	fields := strings.Fields(rest)
	if len(fields) == 0 || !strings.EqualFold(fields[0], name) {
		return "", false
	}
	return strings.TrimSpace(rest[len(fields[0]):]), true
}

func exactNamespace(message, prefix, name string) (string, bool) {
	if prefix == "" || !strings.HasPrefix(message, prefix) {
		return "", false
	}
	rest := message[len(prefix):]
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if isASCIIWhitespace(rest[i]) {
			end = i
			break
		}
	}
	if !strings.EqualFold(rest[:end], name) {
		return "", false
	}
	return trimASCIIWhitespace(rest[end:]), true
}

func trimASCIIWhitespace(value string) string {
	start, end := 0, len(value)
	for start < end && isASCIIWhitespace(value[start]) {
		start++
	}
	for end > start && isASCIIWhitespace(value[end-1]) {
		end--
	}
	return value[start:end]
}

func isASCIIWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}
