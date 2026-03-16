package trivia

import "testing"

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
