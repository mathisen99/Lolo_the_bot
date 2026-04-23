package handler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/database"
	boterrors "github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/user"
)

type noopLogger struct{}

func (noopLogger) Info(string, ...interface{})           {}
func (noopLogger) Success(string, ...interface{})        {}
func (noopLogger) Warning(string, ...interface{})        {}
func (noopLogger) Error(string, ...interface{})          {}
func (noopLogger) ChannelMessage(string, string, string) {}
func (noopLogger) PrivateMessage(string, string)         {}

type testAPIClient struct {
	commandResponses     map[string]string
	streamingResponses   map[string][]*APIResponse
	mentionResponse      string
	mentionPrefix        string
	mentionTriviaContext *TriviaContext
	commandsResponse     *CommandsResponse
}

func (c *testAPIClient) SendCommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (*APIResponse, error) {
	message, ok := c.commandResponses[command]
	if !ok {
		return &APIResponse{RequestID: "api-test", Status: "error", Message: "Unknown command: " + command}, nil
	}
	return &APIResponse{RequestID: "api-test", Status: "success", Message: message}, nil
}

func (c *testAPIClient) SendCommandStream(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (<-chan *APIResponse, error) {
	ch := make(chan *APIResponse, 4)
	go func() {
		defer close(ch)
		if responses, ok := c.streamingResponses[command]; ok {
			for _, response := range responses {
				ch <- response
			}
			return
		}
		if message, ok := c.commandResponses[command]; ok {
			ch <- &APIResponse{RequestID: "api-stream", Status: "success", Message: message}
			return
		}
		ch <- &APIResponse{RequestID: "api-stream", Status: "error", Message: "Unknown command: " + command}
	}()
	return ch, nil
}

func (c *testAPIClient) SendMention(ctx context.Context, message, nick, hostmask, channel, permissionLevel, commandPrefix string, history []*database.Message, triviaContext *TriviaContext, deepMode bool) (*APIResponse, error) {
	c.mentionPrefix = commandPrefix
	c.mentionTriviaContext = triviaContext
	return &APIResponse{RequestID: "mention", Status: "success", Message: c.mentionResponse}, nil
}

func (c *testAPIClient) SendMentionStream(ctx context.Context, message, nick, hostmask, channel, permissionLevel, commandPrefix string, history []*database.Message, triviaContext *TriviaContext, deepMode bool) (<-chan *APIResponse, error) {
	c.mentionPrefix = commandPrefix
	c.mentionTriviaContext = triviaContext
	ch := make(chan *APIResponse, 1)
	go func() {
		defer close(ch)
		ch <- &APIResponse{RequestID: "mention-stream", Status: "success", Message: c.mentionResponse}
	}()
	return ch, nil
}

func (c *testAPIClient) CheckHealth(ctx context.Context) (*HealthResponse, error) {
	return &HealthResponse{Status: "ok", Uptime: 1, Version: "test"}, nil
}

func (c *testAPIClient) GetCommands(ctx context.Context) (*CommandsResponse, error) {
	if c.commandsResponse != nil {
		return c.commandsResponse, nil
	}
	return &CommandsResponse{}, nil
}

func (c *testAPIClient) WaitForInflightRequests(timeout time.Duration) bool {
	return true
}

type handlerTestCommand struct {
	name    string
	help    string
	level   database.PermissionLevel
	execute func(ctx *commands.Context) (*commands.Response, error)
}

func (c *handlerTestCommand) Name() string { return c.name }

func (c *handlerTestCommand) Execute(ctx *commands.Context) (*commands.Response, error) {
	if c.execute != nil {
		return c.execute(ctx)
	}
	return commands.NewResponse(c.name), nil
}

func (c *handlerTestCommand) RequiredPermission() database.PermissionLevel { return c.level }
func (c *handlerTestCommand) Help() string                                 { return c.help }
func (c *handlerTestCommand) CooldownDuration() time.Duration              { return 0 }

type handlerTestEnv struct {
	handler    *MessageHandler
	dispatcher *commands.Dispatcher
	apiClient  *testAPIClient
	db         *database.DB
}

func newHandlerTestEnv(t *testing.T) (*handlerTestEnv, func()) {
	t.Helper()

	db, cleanupDB := database.NewTestDB(t)
	userMgr := user.NewManager(db)
	if err := userMgr.AddUser("admin", "admin@host", database.LevelAdmin); err != nil {
		cleanupDB()
		t.Fatalf("AddUser(admin) failed: %v", err)
	}

	registry := commands.NewRegistry()
	dispatcher := commands.NewDispatcher(registry, userMgr, "!")
	apiClient := &testAPIClient{
		commandResponses:   make(map[string]string),
		streamingResponses: make(map[string][]*APIResponse),
	}

	if err := registry.Register(commands.NewPrefixCommand(db, dispatcher)); err != nil {
		cleanupDB()
		t.Fatalf("Register prefix failed: %v", err)
	}
	if err := registry.Register(&handlerTestCommand{
		name:  "code",
		help:  "!code <lang> - Start code quiz",
		level: database.LevelNormal,
		execute: func(ctx *commands.Context) (*commands.Response, error) {
			if len(ctx.Args) != 1 {
				return nil, boterrors.NewInvalidSyntaxError("code", "!code <lang>")
			}
			return commands.NewResponse("code:" + ctx.Args[0]), nil
		},
	}); err != nil {
		cleanupDB()
		t.Fatalf("Register code failed: %v", err)
	}
	if err := registry.Register(&handlerTestCommand{
		name:  "trivia",
		help:  "!trivia <topic> - Start trivia",
		level: database.LevelNormal,
		execute: func(ctx *commands.Context) (*commands.Response, error) {
			if len(ctx.Args) != 1 {
				return nil, boterrors.NewInvalidSyntaxError("trivia", "!trivia <topic>")
			}
			return commands.NewResponse("trivia:" + ctx.Args[0]), nil
		},
	}); err != nil {
		cleanupDB()
		t.Fatalf("Register trivia failed: %v", err)
	}

	for _, name := range []string{"topic", "topicappend", "channel enable", "user add"} {
		if err := registry.Register(&handlerTestCommand{
			name:  name,
			help:  "!" + name + " - test",
			level: database.LevelNormal,
		}); err != nil {
			cleanupDB()
			t.Fatalf("Register %s failed: %v", name, err)
		}
	}

	if err := registry.Register(commands.NewHelpCommand(registry)); err != nil {
		cleanupDB()
		t.Fatalf("Register help failed: %v", err)
	}

	out, err := output.NewOutput(filepath.Join(t.TempDir(), "errors.log"))
	if err != nil {
		cleanupDB()
		t.Fatalf("NewOutput failed: %v", err)
	}

	handler := NewMessageHandler(&MessageHandlerConfig{
		Dispatcher:            dispatcher,
		APIClient:             apiClient,
		UserManager:           userMgr,
		DB:                    db,
		Logger:                noopLogger{},
		ErrorHandler:          boterrors.NewErrorHandler(out),
		Splitter:              splitter.New(400),
		BotNick:               "Lolo",
		TestMode:              false,
		ImageDownloadChannels: nil,
	})

	cleanup := func() {
		handler.Shutdown()
		cleanupDB()
	}

	return &handlerTestEnv{
		handler:    handler,
		dispatcher: dispatcher,
		apiClient:  apiClient,
		db:         db,
	}, cleanup
}

func TestHandleMessageChannelScopedPrefixOverrides(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	responses, err := env.handler.HandleMessage(ctx, "admin", "admin@host", "#chan1", "!prefix -", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage !prefix - failed: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0], "-prefix !") {
		t.Fatalf("expected prefix change response with reset hint, got %v", responses)
	}

	if got := env.dispatcher.GetActivePrefix("#chan1", false); got != "-" {
		t.Fatalf("expected #chan1 prefix '-', got %q", got)
	}
	if got := env.dispatcher.GetActivePrefix("#chan2", false); got != "!" {
		t.Fatalf("expected #chan2 prefix '!', got %q", got)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan1", "-help", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage -help failed: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0], "-help <command>") {
		t.Fatalf("expected rewritten help response in #chan1, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan1", "!help", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage old prefix failed: %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("expected old prefix to stop working in #chan1, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan2", "!help", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage !help in #chan2 failed: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0], "!help <command>") {
		t.Fatalf("expected default help response in #chan2, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan1", "-code go", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage -code failed: %v", err)
	}
	if len(responses) != 1 || responses[0] != "code:go" {
		t.Fatalf("expected code command response, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan1", "-trivia math", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage -trivia failed: %v", err)
	}
	if len(responses) != 1 || responses[0] != "trivia:math" {
		t.Fatalf("expected trivia command response, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "admin", "admin@host", "#chan1", "-prefix !", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage -prefix ! failed: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0], "!prefix <symbol>") {
		t.Fatalf("expected reset response with default prefix syntax, got %v", responses)
	}

	responses, err = env.handler.HandleMessage(ctx, "alice", "", "#chan1", "!help", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage !help after reset failed: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0], "!help <command>") {
		t.Fatalf("expected default help after reset, got %v", responses)
	}
}

func TestRewriteCommandPrefixesHandlesOverlapsAndMultiWordCommands(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()

	env.dispatcher.SetChannelPrefix("#chan", "-")

	got := env.handler.rewriteCommandPrefixes("Use !topicappend x, !topic y, !channel enable #x, !user add bob, !trivia math.", "#chan", false)
	want := "Use -topicappend x, -topic y, -channel enable #x, -user add bob, -trivia math."
	if got != want {
		t.Fatalf("rewriteCommandPrefixes() = %q, want %q", got, want)
	}
}

func TestHandleAPICommandRewritesKnownCommandExamples(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()

	env.dispatcher.SetChannelPrefix("#chan", "-")
	env.apiClient.commandResponses["weather"] = "Try !trivia math or !topicappend more."

	responses, err := env.handler.HandleMessage(context.Background(), "alice", "", "#chan", "-weather stockholm", false, nil)
	if err != nil {
		t.Fatalf("HandleMessage -weather failed: %v", err)
	}
	if len(responses) != 1 || responses[0] != "Try -trivia math or -topicappend more." {
		t.Fatalf("expected rewritten API response, got %v", responses)
	}
}

func TestHandleMentionRewritesKnownCommandExamplesAndPassesPrefixToAPI(t *testing.T) {
	env, cleanup := newHandlerTestEnv(t)
	defer cleanup()

	env.dispatcher.SetChannelPrefix("#chan", "-")
	env.apiClient.mentionResponse = "Try !trivia math and !help."

	responses, err := env.handler.handleMention(context.Background(), "alice", "", "#chan", "Lolo help me", nil)
	if err != nil {
		t.Fatalf("handleMention failed: %v", err)
	}
	if len(responses) != 1 || responses[0] != "Try -trivia math and -help." {
		t.Fatalf("expected rewritten mention response, got %v", responses)
	}
	if env.apiClient.mentionPrefix != "-" {
		t.Fatalf("expected mention command prefix '-', got %q", env.apiClient.mentionPrefix)
	}
}
