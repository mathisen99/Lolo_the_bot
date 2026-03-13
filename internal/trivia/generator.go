package trivia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

var errMissingAPIKey = errors.New("missing openai api key")

// GeneratorConfig controls OpenAI trivia generation calls.
type GeneratorConfig struct {
	Enabled         bool
	APIKeyEnv       string
	BaseURL         string
	Model           string
	RequestTimeout  time.Duration
	MaxOutputTokens int
}

// Generator calls OpenAI Responses API and validates trivia payloads.
type Generator struct {
	config     GeneratorConfig
	httpClient *http.Client
	logger     output.Logger
}

// NewGenerator builds a trivia question generator.
func NewGenerator(config GeneratorConfig, logger output.Logger) *Generator {
	timeout := config.RequestTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &Generator{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// GenerateQuestion creates a single trivia question for a topic.
func (g *Generator) GenerateQuestion(ctx context.Context, topic string) (*GeneratedQuestion, error) {
	if !g.config.Enabled {
		return nil, ErrGeneratorDisabled
	}

	apiKeyEnv := strings.TrimSpace(g.config.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}

	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return nil, errMissingAPIKey
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(g.config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := strings.TrimSpace(g.config.Model)
	if model == "" {
		model = "gpt-5.2"
	}

	maxOutputTokens := g.config.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = 220
	}

	prompt := buildTriviaPrompt(topic)

	requestBody := map[string]any{
		"model":             model,
		"input":             prompt,
		"max_output_tokens": maxOutputTokens,
		"reasoning": map[string]any{
			"effort": "none",
		},
		"text": map[string]any{
			"verbosity": "low",
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trivia generation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create trivia generation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trivia generation request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read trivia generation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warning("OpenAI trivia generation failed with status %d", resp.StatusCode)
		return nil, fmt.Errorf("openai trivia generation status %d", resp.StatusCode)
	}

	jsonPayload, err := extractTriviaJSON(respBody)
	if err != nil {
		return nil, err
	}

	var question GeneratedQuestion
	if err := json.Unmarshal([]byte(jsonPayload), &question); err != nil {
		return nil, fmt.Errorf("failed to parse trivia JSON payload: %w", err)
	}

	if err := validateGeneratedQuestion(&question); err != nil {
		return nil, err
	}

	return &question, nil
}

func buildTriviaPrompt(topic string) string {
	return fmt.Sprintf(`You are generating one IRC trivia question.
Topic: %s

Return ONLY valid JSON with this exact shape and keys:
{
  "question": "string",
  "answer": "string",
  "aliases": ["string"],
  "hint": "string",
  "uniqueness_key": "string"
}

Rules:
- question must be one concise trivia question.
- answer must be short and factual.
- aliases should include optional alternative exact answers (can be empty array).
- hint must help but not reveal the answer directly.
- uniqueness_key should be short and stable for deduplication.
- Do not include markdown, explanations, or extra keys.
`, strings.TrimSpace(topic))
}

func extractTriviaJSON(rawResponse []byte) (string, error) {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}

	if err := json.Unmarshal(rawResponse, &payload); err != nil {
		return "", fmt.Errorf("failed to parse openai response envelope: %w", err)
	}

	text := strings.TrimSpace(payload.OutputText)
	if text == "" {
		var b strings.Builder
		for _, item := range payload.Output {
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.WriteString(content.Text)
				}
			}
		}
		text = strings.TrimSpace(b.String())
	}

	if text == "" {
		return "", fmt.Errorf("openai trivia response did not include output text")
	}

	jsonText, ok := extractFirstJSONObject(text)
	if !ok {
		return "", fmt.Errorf("openai trivia response did not include a valid JSON object")
	}
	return jsonText, nil
}

func extractFirstJSONObject(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}

	start := strings.IndexByte(trimmed, '{')
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaping := false

	for i := start; i < len(trimmed); i++ {
		ch := trimmed[i]

		if inString {
			if escaping {
				escaping = false
				continue
			}
			if ch == '\\' {
				escaping = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1], true
			}
		}
	}

	return "", false
}

func validateGeneratedQuestion(question *GeneratedQuestion) error {
	question.Question = strings.TrimSpace(question.Question)
	question.Answer = strings.TrimSpace(question.Answer)
	question.Hint = strings.TrimSpace(question.Hint)
	question.UniquenessKey = strings.TrimSpace(question.UniquenessKey)

	if question.Question == "" {
		return fmt.Errorf("invalid trivia payload: question is empty")
	}
	if question.Answer == "" {
		return fmt.Errorf("invalid trivia payload: answer is empty")
	}
	if question.Hint == "" {
		return fmt.Errorf("invalid trivia payload: hint is empty")
	}

	validAliases := make([]string, 0, len(question.Aliases))
	for _, alias := range question.Aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		validAliases = append(validAliases, trimmed)
	}
	question.Aliases = validAliases

	return nil
}
