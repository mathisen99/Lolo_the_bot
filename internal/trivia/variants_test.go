package trivia

import (
	"testing"
	"time"
)

func TestNormalizeCodeLanguageAllowsFreeFormAndAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "py", want: "python", ok: true},
		{input: "Scala", want: "scala", ok: true},
		{input: "common lisp", want: "common lisp", ok: true},
		{input: "F#", want: "f#", ok: true},
		{input: "bad;rm -rf", want: "", ok: false},
	}

	for _, tc := range tests {
		got, ok := NormalizeCodeLanguage(tc.input)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("NormalizeCodeLanguage(%q) = (%q, %t), want (%q, %t)", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBuildTriviaAliasesRealFakeIncludesLabelAndNumber(t *testing.T) {
	t.Parallel()

	metadata, err := marshalTriviaMetadata(RealFakeMetadata{
		Choices: []TriviaChoice{
			{Label: "A", Text: "Mercury is the closest planet to the Sun.", IsTrue: true},
			{Label: "B", Text: "Venus has two moons.", IsTrue: false},
			{Label: "C", Text: "Mars is known as the red planet.", IsTrue: true},
		},
	})
	if err != nil {
		t.Fatalf("marshalTriviaMetadata returned error: %v", err)
	}

	aliases, err := buildTriviaAliases(VariantRealFake, nil, metadata)
	if err != nil {
		t.Fatalf("buildTriviaAliases returned error: %v", err)
	}

	expected := map[string]struct{}{
		NormalizeAnswer("Venus has two moons."): {},
		NormalizeAnswer("B"):                    {},
		NormalizeAnswer("2"):                    {},
	}
	for _, alias := range aliases {
		delete(expected, NormalizeAnswer(alias))
	}
	if len(expected) != 0 {
		t.Fatalf("expected real/fake aliases to include false choice text/label/number, missing=%v", expected)
	}
}

func TestChooseTriviaVariantAvoidsImmediateRepeat(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		if got := chooseTriviaVariant([]string{VariantClassic}); got == VariantClassic {
			t.Fatalf("expected chooseTriviaVariant to avoid immediate repeat, got %q", got)
		}
	}
}

func TestChooseTriviaVariantSoftBlocksRepeatedFamily(t *testing.T) {
	t.Parallel()

	for i := 0; i < 50; i++ {
		got := chooseTriviaVariant([]string{VariantOddOneOut, VariantRealFake})
		if triviaVariantSpec(got).Family == VariantFamilyChoice {
			t.Fatalf("expected chooseTriviaVariant to avoid repeating choice family, got %q", got)
		}
	}
}

func TestIsCorrectAnswerClosestYearAcceptsNumericGuess(t *testing.T) {
	t.Parallel()

	round := &activeRound{
		Mode:    ModeTrivia,
		Variant: VariantClosestYear,
		Answer:  "1999",
	}
	if !isCorrectAnswer(round, "I think 1999") {
		t.Fatalf("expected closest-year round to accept a numeric year guess embedded in text")
	}
	if isCorrectAnswer(round, "2001") {
		t.Fatalf("did not expect wrong year guess to match")
	}
}

func TestGetActiveRoundContextReturnsVisibleSnapshot(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{})
	manager.activeRounds["#test"] = &activeRound{
		Mode:            ModeTrivia,
		Variant:         VariantConnection,
		Channel:         "#test",
		Topic:           "science",
		DisplayQuestion: "What connects these clues? gravity | orbit | mass",
		HintUsed:        true,
	}

	got := manager.GetActiveRoundContext("#test")
	if !got.Active {
		t.Fatalf("expected active round context")
	}
	if got.Variant != VariantConnection || got.Topic != "science" || got.Question == "" || !got.HintUsed {
		t.Fatalf("unexpected active round context: %+v", got)
	}
}

func TestVariantSupportsSpeedOnlyForEligibleModes(t *testing.T) {
	t.Parallel()

	if !variantSupportsSpeed(VariantHigherLower) {
		t.Fatalf("expected higher_lower to support speed")
	}
	if variantSupportsSpeed(VariantClosestNum) {
		t.Fatalf("did not expect closest_number to support speed")
	}
}

func TestIsCorrectAnswerClosestNumberAcceptsExactNumericGuess(t *testing.T) {
	t.Parallel()

	metadata, err := marshalTriviaMetadata(ClosestNumberMetadata{AllowDecimal: true})
	if err != nil {
		t.Fatalf("marshalTriviaMetadata returned error: %v", err)
	}

	round := &activeRound{
		Mode:     ModeTrivia,
		Variant:  VariantClosestNum,
		Answer:   "3.14",
		Metadata: metadata,
	}
	if !isCorrectAnswer(round, "about 3.14") {
		t.Fatalf("expected closest_number round to accept an exact decimal guess embedded in text")
	}
	if isCorrectAnswer(round, "3.1") {
		t.Fatalf("did not expect inexact decimal guess to match")
	}
}

func TestValidateGeneratedQuestionRejectsWrongMetadataShape(t *testing.T) {
	t.Parallel()

	question := &GeneratedQuestion{
		Variant:       VariantChronology,
		Question:      "Order these",
		Answer:        "A-B-C",
		Hint:          "Think oldest to newest.",
		UniquenessKey: "chronology test",
		Metadata:      TriviaQuestionMetadata(`{"choices":[{"label":"A","text":"One"}]}`),
	}

	if err := validateGeneratedQuestion(question, VariantChronology); err == nil {
		t.Fatalf("expected chronology validation to reject wrong metadata shape")
	}
}

func TestFindClosestNumberWinnerUsesEarliestTieBreak(t *testing.T) {
	t.Parallel()

	metadata, err := marshalTriviaMetadata(ClosestNumberMetadata{})
	if err != nil {
		t.Fatalf("marshalTriviaMetadata returned error: %v", err)
	}

	now := time.Now()
	winner := findClosestNumberWinner(&activeRound{
		Variant:  VariantClosestNum,
		Answer:   "100",
		Metadata: metadata,
	}, []GuessLog{
		{ID: 1, Nick: "alice", Message: "98", Timestamp: now},
		{ID: 2, Nick: "bob", Message: "102", Timestamp: now.Add(time.Second)},
	})

	if winner == nil || winner.Nick != "alice" {
		t.Fatalf("expected earliest equally close numeric guess to win, got %+v", winner)
	}
}
