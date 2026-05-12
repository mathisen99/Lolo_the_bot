package trivia

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTriviaJSONFromOutputText(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"output_text": "{\"question\":\"Capital of France?\",\"answer\":\"Paris\",\"aliases\":[\"paris\"],\"hint\":\"City of Light\",\"uniqueness_key\":\"capital france\"}"
	}`)

	jsonPayload, err := extractTriviaJSON(raw)
	if err != nil {
		t.Fatalf("extractTriviaJSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &decoded); err != nil {
		t.Fatalf("failed to unmarshal extracted payload: %v", err)
	}

	if decoded["question"] != "Capital of France?" {
		t.Fatalf("unexpected question: %#v", decoded["question"])
	}
}

func TestResolveResponsesEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "default", base: "", want: "https://api.openai.com/v1/responses"},
		{name: "base url", base: "https://api.openai.com/v1", want: "https://api.openai.com/v1/responses"},
		{name: "api root", base: "https://api.openai.com", want: "https://api.openai.com/v1/responses"},
		{name: "trailing slash", base: "https://api.openai.com/v1/", want: "https://api.openai.com/v1/responses"},
		{name: "full responses endpoint", base: "https://api.openai.com/v1/responses", want: "https://api.openai.com/v1/responses"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveResponsesEndpoint(tc.base); got != tc.want {
				t.Fatalf("resolveResponsesEndpoint(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

func TestDefaultTriviaOpenAIModel(t *testing.T) {
	t.Parallel()

	if defaultTriviaOpenAIModel != "gpt-5.4-nano" {
		t.Fatalf("unexpected default trivia model: %s", defaultTriviaOpenAIModel)
	}
}

func TestExtractTriviaJSONFromFunctionCallArguments(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"output": [
			{
				"type": "function_call",
				"name": "emit_trivia",
				"arguments": "{\"question\":\"Who wrote Hamlet?\",\"answer\":\"Shakespeare\",\"aliases\":[],\"hint\":\"English playwright\",\"uniqueness_key\":\"hamlet author\"}"
			}
		]
	}`)

	jsonPayload, err := extractTriviaJSON(raw)
	if err != nil {
		t.Fatalf("extractTriviaJSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &decoded); err != nil {
		t.Fatalf("failed to unmarshal extracted payload: %v", err)
	}

	if decoded["answer"] != "Shakespeare" {
		t.Fatalf("unexpected answer: %#v", decoded["answer"])
	}
}

func TestExtractTriviaJSONFromLooseKeyValueText(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"output_text": "question: What is 2+2?\nanswer: 4\naliases: four\nhint: Basic arithmetic\nuniqueness_key: two plus two"
	}`)

	jsonPayload, err := extractTriviaJSON(raw)
	if err != nil {
		t.Fatalf("extractTriviaJSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &decoded); err != nil {
		t.Fatalf("failed to unmarshal extracted payload: %v", err)
	}

	if decoded["question"] != "What is 2+2?" {
		t.Fatalf("unexpected question: %#v", decoded["question"])
	}

	aliases, ok := decoded["aliases"].([]any)
	if !ok || len(aliases) != 1 || aliases[0] != "four" {
		t.Fatalf("unexpected aliases: %#v", decoded["aliases"])
	}
}

func TestExtractTriviaJSONMissingOutput(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"status":"completed","output":[]}`)
	_, err := extractTriviaJSON(raw)
	if err == nil {
		t.Fatal("expected error for missing output text")
	}
}

func TestBuildJudgePromptTriviaAllowsUnambiguousShorthand(t *testing.T) {
	t.Parallel()

	req := JudgeRequest{
		Mode:     ModeTrivia,
		Topic:    "pascal",
		Question: `In Pascal, what is a record that uses a "case" section to store different field layouts called?`,
		Answer:   "A variant record (record with a variant part).",
		Candidates: []JudgeGuessCandidate{
			{ID: 1, Nick: "joanna", Guess: "variant", ElapsedMS: 23000},
		},
	}

	prompt, err := buildJudgePrompt(req)
	if err != nil {
		t.Fatalf("buildJudgePrompt returned error: %v", err)
	}

	if !strings.Contains(prompt, "concise shorthand when meaning is unambiguous") {
		t.Fatalf("expected prompt to include shorthand acceptance guidance")
	}
}

func TestBuildAntiCheatPromptScopesActiveRoundAndIgnoredUsers(t *testing.T) {
	t.Parallel()

	req := AntiCheatRequest{
		Mode:          ModeTrivia,
		Variant:       VariantClassic,
		Topic:         "geography",
		Question:      "What is the capital of France?",
		Answer:        "Paris",
		WinnerNick:    "alice",
		WinningAnswer: "Paris",
		Observations: []AntiCheatObservation{
			{ID: 1, Nick: "ignored", Message: "the answer is Paris", ElapsedMS: 1200, Ignored: true},
			{ID: 2, Nick: "alice", Message: "Paris", ElapsedMS: 2000, Ignored: false},
		},
	}

	prompt, err := buildAntiCheatPrompt(req)
	if err != nil {
		t.Fatalf("buildAntiCheatPrompt returned error: %v", err)
	}
	for _, want := range []string{
		"Only evaluate public same-channel messages from the active round",
		"Messages before/after the round, private messages, and other channels are out of scope",
		`"ignored":true`,
		"help or answer leaks from them should count",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q", want)
		}
	}
}
