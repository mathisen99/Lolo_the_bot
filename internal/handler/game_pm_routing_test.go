package handler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	boterrors "github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/game"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/user"
)

type failPMLogger struct{ noopLogger }

func (failPMLogger) PrivateMessage(string, string) {
	panic("ordinary PM logging must be unreachable")
}

type failPMAPI struct{}

func (*failPMAPI) SendCommand(context.Context, string, []string, string, string, string, string, bool, time.Duration) (*APIResponse, error) {
	panic("generic /command must be unreachable")
}
func (*failPMAPI) SendCommandStream(context.Context, string, []string, string, string, string, string, bool, time.Duration) (<-chan *APIResponse, error) {
	panic("generic /command/stream must be unreachable")
}
func (*failPMAPI) SendMention(context.Context, string, string, string, string, string, string, string, []*database.Message) (*APIResponse, error) {
	panic("generic /mention, LLM chat, and tools must be unreachable")
}
func (*failPMAPI) SendMentionStream(context.Context, string, string, string, string, string, string, string, []*database.Message) (<-chan *APIResponse, error) {
	panic("generic /mention/stream, LLM chat, and tools must be unreachable")
}
func (*failPMAPI) CheckHealth(context.Context) (*HealthResponse, error) {
	return &HealthResponse{Status: "ok"}, nil
}
func (*failPMAPI) GetCommands(context.Context) (*CommandsResponse, error) {
	return &CommandsResponse{}, nil
}
func (*failPMAPI) WaitForInflightRequests(time.Duration) bool { return true }

type pmGameSpy struct {
	calls []game.PMGameInput
}

func (s *pmGameSpy) HandlePMGame(_ context.Context, input game.PMGameInput) ([]game.Delivery, error) {
	s.calls = append(s.calls, input)
	return []game.Delivery{{Target: game.DeliveryPM, Lines: []string{"game-route"}}}, nil
}

type pmContinuationSpy struct{}

func (pmContinuationSpy) ResolvePMContinuation(_ game.ContinuationKey, input string, _ time.Time) (game.ContinuationBinding, game.ContinuationStatus) {
	switch input {
	case "attack":
		return game.ContinuationBinding{Input: input, Action: "attack", MenuContextID: "current", StateRevision: 7}, game.ContinuationCurrent
	case "c-stale":
		return game.ContinuationBinding{}, game.ContinuationStale
	case "c-expired":
		return game.ContinuationBinding{}, game.ContinuationExpired
	case "c-ambiguous":
		return game.ContinuationBinding{}, game.ContinuationAmbiguous
	default:
		return game.ContinuationBinding{}, game.ContinuationUnknown
	}
}

type panicCommand struct{ name string }

func (c panicCommand) Name() string { return c.name }
func (c panicCommand) Execute(*commands.Context) (*commands.Response, error) {
	panic("general Go command dispatcher must be unreachable")
}
func (panicCommand) RequiredPermission() database.PermissionLevel { return database.LevelNormal }
func (panicCommand) Help() string                                 { return "unreachable" }
func (panicCommand) CooldownDuration() time.Duration              { return 0 }

func newPMBoundaryHandler(t *testing.T, pmEnabled bool, setPassword bool) (*MessageHandler, *pmGameSpy, *user.Manager, *database.DB, func()) {
	t.Helper()
	db, cleanupDB := database.NewTestDB(t)
	if err := db.SetPMState(pmEnabled); err != nil {
		cleanupDB()
		t.Fatalf("SetPMState failed: %v", err)
	}
	users := user.NewManager(db)
	if setPassword {
		if err := users.SetOwnerPassword("sword fish"); err != nil {
			cleanupDB()
			t.Fatalf("SetOwnerPassword failed: %v", err)
		}
	}
	registry := commands.NewRegistry()
	verify := commands.NewVerifyCommand(users, db)
	if err := registry.Register(verify); err != nil {
		cleanupDB()
		t.Fatalf("register verify: %v", err)
	}
	if err := registry.Register(panicCommand{name: "ping"}); err != nil {
		cleanupDB()
		t.Fatalf("register ping: %v", err)
	}
	dispatcher := commands.NewDispatcherForNetwork(registry, users, "!", "libera", func(string) (bool, error) {
		panic("owner permission verification must not be reached by rejected PM input")
	})
	gameSpy := &pmGameSpy{}
	gameCfg := config.GameConfig{
		Enabled: true, Command: "avenger", PMEnabled: true,
		PMRejectMode: "help", MaxInputBytes: 64,
	}
	router := game.NewRouter(
		game.RouterConfigFrom("libera", "!", gameCfg),
		game.RouterDependencies{
			Users: users, PMState: db, Verify: verify, Game: gameSpy,
			Identities: game.NewIdentityResolver(users, game.NewCaseMappingStore(), game.RegisteredIdentityPolicyFunc(
				func(context.Context, string, string, string, *database.User) (bool, error) {
					panic("registered identity policy must not run for unregistered PM input")
				},
			)),
			Continuations: pmContinuationSpy{},
		},
	)
	out, err := output.NewOutput(filepath.Join(t.TempDir(), "errors.log"))
	if err != nil {
		cleanupDB()
		t.Fatalf("NewOutput: %v", err)
	}
	h := NewMessageHandler(&MessageHandlerConfig{
		Network: "libera", Dispatcher: dispatcher, APIClient: &failPMAPI{},
		UserManager: users, DB: db, Logger: failPMLogger{},
		ErrorHandler: boterrors.NewErrorHandler(out), Splitter: splitter.New(400),
		BotNick: "Lolo", GameRouter: router,
	})
	return h, gameSpy, users, db, func() { h.Shutdown(); cleanupDB() }
}

func TestPMRoutingPrecedenceAndTerminalIsolation(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		pmEnabled    bool
		setPassword  bool
		wantGame     bool
		wantCommand  string
		wantContinue string
		wantResponse string
		wantOwner    bool
	}{
		{name: "malformed verify stays in VerifyCommand", message: "!verify", wantResponse: "Usage: !verify <password>"},
		{name: "near verify token is rejected", message: "!verifyx sword fish", wantResponse: "Game PM syntax:"},
		{name: "game namespace only", message: "!avenger start", wantGame: true, wantCommand: "start", wantResponse: "game-route"},
		{name: "current continuation only", message: "attack", wantGame: true, wantContinue: "attack", wantResponse: "game-route"},
		{name: "stale continuation", message: "c-stale", wantResponse: "Game PM syntax:"},
		{name: "expired continuation", message: "c-expired", wantResponse: "Game PM syntax:"},
		{name: "ambiguous continuation", message: "c-ambiguous", wantResponse: "Game PM syntax:"},
		{name: "unknown continuation", message: "defend", wantResponse: "Game PM syntax:"},
		{name: "unrelated registered Go command", message: "!ping", wantResponse: "Game PM syntax:"},
		{name: "unrelated Python command", message: "!echo hello", wantResponse: "Game PM syntax:"},
		{name: "mention", message: "Lolo: explain this", wantResponse: "Game PM syntax:"},
		{name: "arbitrary LLM-shaped text", message: "write me a detailed story", wantResponse: "Game PM syntax:"},
		{name: "streaming command path", message: "!stream_example", wantResponse: "Game PM syntax:"},
		{name: "non-game tool request", message: "use web_search to find this", wantResponse: "Game PM syntax:"},
		{name: "existing PM policy disables game", message: "!avenger start", pmEnabled: false, wantResponse: "Game PM syntax:"},
		{name: "oversized input is rejected before parsing", message: strings.Repeat("x", 65), wantResponse: "Game PM syntax:"},
		{name: "valid verify uses existing handler only", message: "!verify sword fish", setPassword: true, wantResponse: "Owner verified!", wantOwner: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pmEnabled := tc.pmEnabled
			if tc.name != "existing PM policy disables game" {
				pmEnabled = true
			}
			h, gameSpy, users, db, cleanup := newPMBoundaryHandler(t, pmEnabled, tc.setPassword)
			defer cleanup()

			responses, err := h.HandleMessage(context.Background(), "alice", "ident@untrusted.example", "", tc.message, true, func(string) {
				panic("PM status/stream callback must be unreachable")
			})
			if err != nil {
				t.Fatalf("HandleMessage failed: %v", err)
			}
			joined := strings.Join(responses, "\n")
			if !strings.Contains(joined, tc.wantResponse) {
				t.Fatalf("response %q does not contain %q", joined, tc.wantResponse)
			}
			if got := len(gameSpy.calls); got != boolCount(tc.wantGame) {
				t.Fatalf("game calls = %d, want %d", got, boolCount(tc.wantGame))
			}
			if tc.wantGame {
				call := gameSpy.calls[0]
				if call.CommandText != tc.wantCommand {
					t.Fatalf("command text = %q, want %q", call.CommandText, tc.wantCommand)
				}
				if tc.wantContinue != "" && (call.Continuation == nil || call.Continuation.Input != tc.wantContinue) {
					t.Fatalf("continuation = %#v, want %q", call.Continuation, tc.wantContinue)
				}
				if strings.Contains(call.CommandText, "sword fish") {
					t.Fatal("verification password entered the game route")
				}
			}
			owner, err := users.GetOwner()
			if err != nil {
				t.Fatalf("GetOwner: %v", err)
			}
			if (owner != nil) != tc.wantOwner {
				t.Fatalf("owner present = %v, want %v", owner != nil, tc.wantOwner)
			}
			if size := users.GetWhoisCacheSize(); size != 0 {
				t.Fatalf("unregistered PM route populated WHOIS state: %d", size)
			}
			// The first PM branch must bypass ordinary message persistence too,
			// especially for verification passwords.
			messages, err := db.QueryMessages(&database.MessageFilter{Network: "libera", Nick: "alice", Limit: 20})
			if err == nil {
				for _, message := range messages {
					if strings.Contains(message.Content, "sword fish") {
						t.Fatal("verification password entered ordinary message records")
					}
				}
			}
		})
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestPMRoutingIgnoredUserIsSilentWithoutGameSideEffects(t *testing.T) {
	h, gameSpy, users, _, cleanup := newPMBoundaryHandler(t, true, false)
	defer cleanup()
	if err := users.AddUser("ignored", "ignored@host", database.LevelIgnored); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	responses, err := h.HandleMessage(context.Background(), "ignored", "ignored@host", "", "!avenger start", true, nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(responses) != 0 || len(gameSpy.calls) != 0 {
		t.Fatalf("ignored route was not silent: responses=%v calls=%d", responses, len(gameSpy.calls))
	}
}
