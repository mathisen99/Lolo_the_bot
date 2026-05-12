package trivia

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type triviaNoopLogger struct{}

func (triviaNoopLogger) Info(string, ...interface{})           {}
func (triviaNoopLogger) Success(string, ...interface{})        {}
func (triviaNoopLogger) Warning(string, ...interface{})        {}
func (triviaNoopLogger) Error(string, ...interface{})          {}
func (triviaNoopLogger) ChannelMessage(string, string, string) {}
func (triviaNoopLogger) PrivateMessage(string, string)         {}

func TestBuildAcceptedAnswerSetTriviaStaysExactWithoutAliases(t *testing.T) {
	answer := "A variant record (record with a variant part)."
	accepted, normalizedAnswer, _ := buildAcceptedAnswerSet(ModeTrivia, answer, nil)

	if normalizedAnswer != NormalizeAnswer(answer) {
		t.Fatalf("unexpected normalized answer: got %q want %q", normalizedAnswer, NormalizeAnswer(answer))
	}

	if _, ok := accepted[NormalizeAnswer(answer)]; !ok {
		t.Fatalf("expected accepted answers to include canonical normalized answer")
	}

	round := &activeRound{
		Mode:            ModeTrivia,
		AcceptedAnswers: accepted,
	}
	if isCorrectAnswer(round, "Variant") {
		t.Fatalf("did not expect shorthand answer to be accepted without alias or judge")
	}
}

func TestShouldRunTimeoutJudgeRunsForAnyTriviaWithGuesses(t *testing.T) {
	manager := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: true},
		},
	}
	round := &activeRound{Mode: ModeTrivia, Variant: VariantClassic}
	guesses := []GuessLog{{ID: 1, Nick: "joanna", Message: "variant"}}

	if !manager.shouldRunTimeoutJudge(round, guesses) {
		t.Fatalf("expected timeout judge to run for trivia rounds with guesses")
	}
}

func TestShouldRunTimeoutJudgeSkipsChoiceOnlyVariants(t *testing.T) {
	manager := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: true},
		},
	}
	round := &activeRound{Mode: ModeTrivia, Variant: VariantRealFake}
	guesses := []GuessLog{{ID: 1, Nick: "joanna", Message: "B"}}

	if manager.shouldRunTimeoutJudge(round, guesses) {
		t.Fatalf("did not expect timeout judge to run for non-semantic choice variants")
	}
}

func TestShouldRunTimeoutJudgeRequiresEnabledGeneratorAndGuesses(t *testing.T) {
	guesses := []GuessLog{{ID: 1, Message: "x"}}

	disabled := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: false},
		},
	}
	round := &activeRound{Mode: ModeTrivia, Variant: VariantClassic}
	if disabled.shouldRunTimeoutJudge(round, guesses) {
		t.Fatalf("did not expect judge to run when generator is disabled")
	}

	enabledNoGuesses := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: true},
		},
	}
	if enabledNoGuesses.shouldRunTimeoutJudge(round, nil) {
		t.Fatalf("did not expect judge to run without guesses")
	}

	noGenerator := &Manager{}
	if noGenerator.shouldRunTimeoutJudge(round, guesses) {
		t.Fatalf("did not expect judge to run without generator")
	}
}

func TestGetLastTriviaTopicAndCodeLanguageEmptyByDefault(t *testing.T) {
	manager := NewManager(ManagerConfig{})

	if topic, ok := manager.GetLastTriviaTopic("#test"); ok || topic != "" {
		t.Fatalf("expected no remembered trivia topic, got ok=%t topic=%q", ok, topic)
	}

	if language, ok := manager.GetLastCodeLanguage("#test"); ok || language != "" {
		t.Fatalf("expected no remembered code language, got ok=%t language=%q", ok, language)
	}
}

func TestStartRoundRemembersLastTriviaTopicEvenWhenGenerationUnavailable(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)

	_, err := manager.StartRound(context.Background(), "#test", "  history  ")
	if !errors.Is(err, ErrGeneratorDisabled) {
		t.Fatalf("expected ErrGeneratorDisabled, got %v", err)
	}

	topic, ok := manager.GetLastTriviaTopic("#test")
	if !ok {
		t.Fatalf("expected remembered trivia topic")
	}
	if topic != "history" {
		t.Fatalf("unexpected remembered trivia topic: got %q want %q", topic, "history")
	}
}

func TestStartCodeRoundRemembersCanonicalLanguageEvenWhenGenerationUnavailable(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)

	_, err := manager.StartCodeRound(context.Background(), "#test", "py")
	if !errors.Is(err, ErrGeneratorDisabled) {
		t.Fatalf("expected ErrGeneratorDisabled, got %v", err)
	}

	language, ok := manager.GetLastCodeLanguage("#test")
	if !ok {
		t.Fatalf("expected remembered code language")
	}
	if language != "python" {
		t.Fatalf("unexpected remembered code language: got %q want %q", language, "python")
	}
}

func TestExactAnswerRunsAntiCheatBeforeAward(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)
	startSimpleRound(t, manager, "#test", true)

	called := false
	manager.antiCheatJudge = func(ctx context.Context, req AntiCheatRequest) (*AntiCheatDecision, error) {
		called = true
		if req.WinnerNick != "alice" {
			t.Fatalf("unexpected winner nick: %s", req.WinnerNick)
		}
		if len(req.Observations) == 0 {
			t.Fatalf("expected active-round observations")
		}
		return &AntiCheatDecision{Cheated: false, Confidence: 0.95, Reason: "clean"}, nil
	}

	manager.ObserveMessage("#test", "alice", "Paris", false)
	response, handled, err := manager.TryAnswer("#test", "alice", "Paris")
	if err != nil {
		t.Fatalf("TryAnswer returned error: %v", err)
	}
	if !handled {
		t.Fatalf("expected exact answer to be handled")
	}
	if !called {
		t.Fatalf("expected anti-cheat judge to run before award")
	}
	if !strings.Contains(response, "alice got it") {
		t.Fatalf("unexpected response: %s", response)
	}

	score, found, err := manager.GetScore("#test", "alice")
	if err != nil {
		t.Fatalf("GetScore returned error: %v", err)
	}
	if !found || score <= 0 {
		t.Fatalf("expected awarded score, got found=%t score=%d", found, score)
	}
}

func TestAntiCheatDisqualificationPenalizesChannelScore(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)
	if err := manager.SetScore("#test", "alice", 51); err != nil {
		t.Fatalf("SetScore returned error: %v", err)
	}
	startSimpleRound(t, manager, "#test", true)

	manager.antiCheatJudge = func(ctx context.Context, req AntiCheatRequest) (*AntiCheatDecision, error) {
		if len(req.Observations) < 2 {
			t.Fatalf("expected ignored helper and winner observations, got %d", len(req.Observations))
		}
		if req.Observations[0].Nick != "ignored" || !req.Observations[0].Ignored {
			t.Fatalf("expected ignored helper observation first, got %#v", req.Observations[0])
		}
		return &AntiCheatDecision{
			Cheated:    true,
			Confidence: 0.96,
			Reason:     "ignored user supplied the answer during the round",
			HelperNicks: []string{
				"ignored",
			},
			EvidenceIDs: []int{req.Observations[0].ID, req.Observations[1].ID},
		}, nil
	}

	manager.ObserveMessage("#test", "ignored", "the answer is Paris", true)
	manager.ObserveMessage("#test", "alice", "Paris", false)
	response, handled, err := manager.TryAnswer("#test", "alice", "Paris")
	if err != nil {
		t.Fatalf("TryAnswer returned error: %v", err)
	}
	if !handled {
		t.Fatalf("expected exact answer to be handled")
	}
	if !strings.Contains(response, "Anti-cheat disqualified alice") || !strings.Contains(response, "Penalty: -11 points (total: 40)") {
		t.Fatalf("unexpected disqualification response: %s", response)
	}

	score, found, err := manager.GetScore("#test", "alice")
	if err != nil {
		t.Fatalf("GetScore returned error: %v", err)
	}
	if !found || score != 40 {
		t.Fatalf("expected penalized score 40, got found=%t score=%d", found, score)
	}
}

func TestAntiCheatDisabledSkipsJudge(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)
	startSimpleRound(t, manager, "#test", false)

	manager.antiCheatJudge = func(ctx context.Context, req AntiCheatRequest) (*AntiCheatDecision, error) {
		t.Fatalf("anti-cheat judge should not run when disabled")
		return nil, nil
	}

	manager.ObserveMessage("#test", "ignored", "the answer is Paris", true)
	manager.ObserveMessage("#test", "alice", "Paris", false)
	response, handled, err := manager.TryAnswer("#test", "alice", "Paris")
	if err != nil {
		t.Fatalf("TryAnswer returned error: %v", err)
	}
	if !handled || !strings.Contains(response, "alice got it") {
		t.Fatalf("expected normal award response, got handled=%t response=%q", handled, response)
	}
}

func TestAntiCheatJudgeFailureAwardsNormally(t *testing.T) {
	manager := newTriviaManagerForRememberTests(t)
	startSimpleRound(t, manager, "#test", true)

	manager.antiCheatJudge = func(ctx context.Context, req AntiCheatRequest) (*AntiCheatDecision, error) {
		return nil, errors.New("judge unavailable")
	}

	manager.ObserveMessage("#test", "alice", "Paris", false)
	response, handled, err := manager.TryAnswer("#test", "alice", "Paris")
	if err != nil {
		t.Fatalf("TryAnswer returned error: %v", err)
	}
	if !handled || !strings.Contains(response, "alice got it") {
		t.Fatalf("expected normal award after fail-open, got handled=%t response=%q", handled, response)
	}
}

func newTriviaManagerForRememberTests(t *testing.T) *Manager {
	t.Helper()

	store, err := NewStore(":memory:", StoreDefaults{
		Settings: ChannelSettings{
			AnswerTimeSeconds:     30,
			CodeAnswerTimeSeconds: 30,
			TriviaHintsEnabled:    true,
			CodeHintsEnabled:      true,
			AntiCheatEnabled:      true,
			BasePoints:            10,
			MinimumPoints:         1,
			HintPenalty:           2,
			Enabled:               true,
			Difficulty:            DifficultyMedium,
			CodeDifficulty:        DifficultyMedium,
		},
	})
	if err != nil {
		t.Fatalf("failed to create in-memory trivia store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return NewManager(ManagerConfig{
		Store:  store,
		Logger: triviaNoopLogger{},
	})
}

func startSimpleRound(t *testing.T, manager *Manager, channel string, antiCheatEnabled bool) {
	t.Helper()

	question := &StoredQuestion{
		ID:            1,
		Mode:          ModeTrivia,
		Variant:       VariantClassic,
		Topic:         "geography",
		Question:      "What is the capital of France?",
		Answer:        "Paris",
		Aliases:       []string{"paris"},
		Hint:          "City of Light",
		Metadata:      TriviaQuestionMetadata(`{}`),
		ValidatorType: ValidatorNormalizedExact,
		CreatedAt:     time.Now(),
	}
	settings := ChannelSettings{
		AnswerTimeSeconds:     30,
		CodeAnswerTimeSeconds: 30,
		TriviaHintsEnabled:    true,
		CodeHintsEnabled:      true,
		AntiCheatEnabled:      antiCheatEnabled,
		BasePoints:            100,
		MinimumPoints:         20,
		HintPenalty:           20,
		Enabled:               true,
		Difficulty:            DifficultyMedium,
		CodeDifficulty:        DifficultyMedium,
	}
	if _, err := manager.startRoundFromStoredQuestion(channel, settings, ModeTrivia, "geography", "", question); err != nil {
		t.Fatalf("startRoundFromStoredQuestion returned error: %v", err)
	}
}
