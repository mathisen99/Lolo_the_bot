package trivia

import (
	"encoding/json"
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
