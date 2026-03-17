package trivia

import (
	"context"
	"errors"
	"testing"
)

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
	guesses := []GuessLog{{ID: 1, Nick: "joanna", Message: "variant"}}

	if !manager.shouldRunTimeoutJudge(guesses) {
		t.Fatalf("expected timeout judge to run for trivia rounds with guesses")
	}
}

func TestShouldRunTimeoutJudgeRequiresEnabledGeneratorAndGuesses(t *testing.T) {
	guesses := []GuessLog{{ID: 1, Message: "x"}}

	disabled := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: false},
		},
	}
	if disabled.shouldRunTimeoutJudge(guesses) {
		t.Fatalf("did not expect judge to run when generator is disabled")
	}

	enabledNoGuesses := &Manager{
		generator: &Generator{
			config: GeneratorConfig{Enabled: true},
		},
	}
	if enabledNoGuesses.shouldRunTimeoutJudge(nil) {
		t.Fatalf("did not expect judge to run without guesses")
	}

	noGenerator := &Manager{}
	if noGenerator.shouldRunTimeoutJudge(guesses) {
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

func newTriviaManagerForRememberTests(t *testing.T) *Manager {
	t.Helper()

	store, err := NewStore(":memory:", StoreDefaults{
		Settings: ChannelSettings{
			AnswerTimeSeconds:     30,
			CodeAnswerTimeSeconds: 30,
			HintsEnabled:          true,
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
		Store: store,
	})
}
