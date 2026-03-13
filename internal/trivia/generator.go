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
	ReasoningEffort string
	RequestTimeout  time.Duration
	MaxOutputTokens int
}

// Generator calls OpenAI Responses API and validates trivia payloads.
type Generator struct {
	config     GeneratorConfig
	httpClient *http.Client
	logger     output.Logger
}

const (
	maxAnswerLength      = 160
	maxAliasLength       = 160
	maxCodeQuestionLen   = 220
	maxCodeAnswerLength  = 280
	maxCodeAliasLength   = 280
	maxCodeHintLength    = 200
	maxCodeUniqueKeyLen  = 180
	maxCodeLanguageField = 32
)

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

// GenerateQuestion creates a single trivia question for a topic and difficulty.
// avoidKeys contains recently rejected normalized dedup keys for this generation cycle.
// avoidQuestions contains recent historical question texts to avoid repeating/paraphrasing.
func (g *Generator) GenerateQuestion(ctx context.Context, topic, difficulty string, attempt int, avoidKeys, avoidQuestions []string) (*GeneratedQuestion, error) {
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

	prompt := buildTriviaPrompt(topic, difficulty, attempt, avoidKeys, avoidQuestions)
	reasoningEffort := normalizeReasoningEffort(g.config.ReasoningEffort)

	requestBody := map[string]any{
		"model":             model,
		"input":             prompt,
		"max_output_tokens": maxOutputTokens,
		"reasoning": map[string]any{
			"effort": reasoningEffort,
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

// GenerateCodeQuestion creates a one-line coding question for a specific language.
func (g *Generator) GenerateCodeQuestion(ctx context.Context, language string, attempt int, avoidKeys, avoidQuestions []string) (*GeneratedCodeQuestion, error) {
	if !g.config.Enabled {
		return nil, ErrGeneratorDisabled
	}

	canonicalLanguage, ok := NormalizeCodeLanguage(language)
	if !ok {
		return nil, ErrUnsupportedCodeLanguage
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

	prompt := buildCodePrompt(canonicalLanguage, attempt, avoidKeys, avoidQuestions)

	requestBody := map[string]any{
		"model":             model,
		"input":             prompt,
		"max_output_tokens": maxOutputTokens,
		"reasoning": map[string]any{
			"effort": normalizeReasoningEffort(g.config.ReasoningEffort),
		},
		"text": map[string]any{
			"verbosity": "low",
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal code generation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create code generation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("code generation request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read code generation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warning("OpenAI code generation failed with status %d", resp.StatusCode)
		return nil, fmt.Errorf("openai code generation status %d", resp.StatusCode)
	}

	jsonPayload, err := extractTriviaJSON(respBody)
	if err != nil {
		return nil, err
	}

	var question GeneratedCodeQuestion
	if err := json.Unmarshal([]byte(jsonPayload), &question); err != nil {
		return nil, fmt.Errorf("failed to parse code JSON payload: %w", err)
	}

	if err := validateGeneratedCodeQuestion(&question, canonicalLanguage); err != nil {
		return nil, err
	}

	return &question, nil
}

// JudgeClosestGuess runs strict post-timeout judging for long-form answers.
func (g *Generator) JudgeClosestGuess(ctx context.Context, req JudgeRequest) (*JudgeDecision, error) {
	if !g.config.Enabled {
		return nil, ErrGeneratorDisabled
	}
	if strings.TrimSpace(req.Answer) == "" {
		return nil, fmt.Errorf("judge request missing canonical answer")
	}
	if len(req.Candidates) == 0 {
		return nil, nil
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

	prompt, err := buildJudgePrompt(req)
	if err != nil {
		return nil, err
	}

	requestBody := map[string]any{
		"model":             model,
		"input":             prompt,
		"max_output_tokens": 220,
		"reasoning": map[string]any{
			"effort": normalizeReasoningEffort(g.config.ReasoningEffort),
		},
		"text": map[string]any{
			"verbosity": "low",
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trivia judge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create trivia judge request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("trivia judge request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read trivia judge response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warning("OpenAI trivia judge failed with status %d", resp.StatusCode)
		return nil, fmt.Errorf("openai trivia judge status %d", resp.StatusCode)
	}

	jsonPayload, err := extractTriviaJSON(respBody)
	if err != nil {
		return nil, err
	}

	var decision JudgeDecision
	if err := json.Unmarshal([]byte(jsonPayload), &decision); err != nil {
		return nil, fmt.Errorf("failed to parse trivia judge JSON payload: %w", err)
	}
	if err := validateJudgeDecision(&decision, req.Candidates); err != nil {
		return nil, err
	}

	return &decision, nil
}

func buildTriviaPrompt(topic, difficulty string, attempt int, avoidKeys, avoidQuestions []string) string {
	now := time.Now().UTC().Format("2006-01-02")
	difficulty = NormalizeDifficulty(difficulty)
	var prompt strings.Builder
	_, _ = fmt.Fprintf(&prompt, `You are generating one IRC trivia question.
Topic: %s
Difficulty: %s
Current date (UTC): %s

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
- answer must be factual and concise when possible, but can be longer if accuracy requires it.
- aliases should include optional alternative exact answers (can be empty array).
- hint must help but not reveal the answer directly.
- uniqueness_key should be short and stable for deduplication.
- The question must be materially different from previous questions and facts.
- Use up-to-date, modern, canonical terminology and facts as of the current date.
- Avoid deprecated/legacy aliases unless the question explicitly asks about legacy behavior.
- If a fact is version-dependent or likely outdated, avoid it and choose a more stable current fact.
- For SQLite metadata questions, prefer canonical 'sqlite_schema' over legacy alias-only answers.
- Do not include markdown, explanations, or extra keys.
`, strings.TrimSpace(topic), difficulty, now)

	_, _ = fmt.Fprintf(&prompt, "\nDifficulty requirements (%s):\n%s\n", difficulty, difficultyPromptGuidance(difficulty))

	// Encourage variation so retry attempts do not repeat deterministic questions.
	_, _ = fmt.Fprintf(&prompt, "\nGeneration attempt: %d", attempt)
	_, _ = fmt.Fprintf(&prompt, "\nVariation nonce: %d", time.Now().UnixNano())

	if len(avoidKeys) > 0 {
		prompt.WriteString("\nDo not repeat or paraphrase these previously rejected uniqueness keys in this generation cycle:\n")
		for _, key := range avoidKeys {
			if strings.TrimSpace(key) == "" {
				continue
			}
			prompt.WriteString("- ")
			prompt.WriteString(key)
			prompt.WriteByte('\n')
		}
	}

	if len(avoidQuestions) > 0 {
		prompt.WriteString("\nDo not repeat or paraphrase any of these recent trivia questions:\n")
		for _, question := range avoidQuestions {
			clean := strings.Join(strings.Fields(strings.TrimSpace(question)), " ")
			if clean == "" {
				continue
			}
			if len(clean) > 220 {
				clean = clean[:220] + "..."
			}
			prompt.WriteString("- ")
			prompt.WriteString(clean)
			prompt.WriteByte('\n')
		}
	}

	return prompt.String()
}

func buildCodePrompt(language string, attempt int, avoidKeys, avoidQuestions []string) string {
	language = strings.TrimSpace(language)

	var prompt strings.Builder
	_, _ = fmt.Fprintf(&prompt, `Generate a one-line coding quiz for IRC.

Language: %s

Requirements:
- Return JSON only
- No markdown
- No explanations
- The task must be solvable with exactly one line of code in %s
- The answer must fit in one IRC message
- No newline characters in the answer
- Prefer expressions over statements
- Do not require multi-line syntax, indentation, class definitions, or full functions
- Only generate a question with one clear expected answer or a very small set of equivalent answers
- Include a short hint
- Keep the question concise and unambiguous

Return exactly this JSON schema:
{
  "language": "%s",
  "question": "string",
  "answer": "string",
  "aliases": ["string"],
  "hint": "string",
  "uniqueness_key": "string",
  "validator_type": "normalized_exact"
}
`, language, language, language)

	_, _ = fmt.Fprintf(&prompt, "\nGeneration attempt: %d", attempt)
	_, _ = fmt.Fprintf(&prompt, "\nVariation nonce: %d", time.Now().UnixNano())

	if len(avoidKeys) > 0 {
		prompt.WriteString("\nDo not repeat or paraphrase these previously rejected uniqueness keys in this generation cycle:\n")
		for _, key := range avoidKeys {
			if strings.TrimSpace(key) == "" {
				continue
			}
			prompt.WriteString("- ")
			prompt.WriteString(key)
			prompt.WriteByte('\n')
		}
	}

	if len(avoidQuestions) > 0 {
		prompt.WriteString("\nDo not repeat or paraphrase any of these recent coding quiz questions:\n")
		for _, question := range avoidQuestions {
			clean := strings.Join(strings.Fields(strings.TrimSpace(question)), " ")
			if clean == "" {
				continue
			}
			if len(clean) > 220 {
				clean = clean[:220] + "..."
			}
			prompt.WriteString("- ")
			prompt.WriteString(clean)
			prompt.WriteByte('\n')
		}
	}

	return prompt.String()
}

func buildJudgePrompt(req JudgeRequest) (string, error) {
	aliasesJSON, err := json.Marshal(req.Aliases)
	if err != nil {
		return "", fmt.Errorf("failed to marshal judge aliases: %w", err)
	}
	candidatesJSON, err := json.Marshal(req.Candidates)
	if err != nil {
		return "", fmt.Errorf("failed to marshal judge candidates: %w", err)
	}

	var prompt strings.Builder
	switch NormalizeMode(req.Mode) {
	case ModeCode:
		_, _ = fmt.Fprintf(&prompt, `You are a strict IRC code-quiz judge.
Language: %s
Question: %s
Official one-line code answer: %s
Accepted aliases (JSON): %s

Candidate guesses (chronological, JSON):
%s

Decide if any candidate guess should be treated as correct.
Strict judging rules:
- First priority is exact meaning and valid one-line code for the requested language.
- Allow tiny typos or trivial formatting mistakes only if intent and code semantics are clearly the same.
- Reject guesses that change meaning, are incomplete, use the wrong language style, or are too vague.
- If multiple candidates qualify, pick the earliest one (lowest elapsed_ms).
- If none qualify, approved must be false.

Return ONLY JSON with this exact shape:
{
  "approved": true or false,
  "guess_id": integer (0 if approved is false),
  "confidence": number between 0 and 1,
  "reason": "short explanation"
}
`, strings.TrimSpace(req.Language), strings.TrimSpace(req.Question), strings.TrimSpace(req.Answer), string(aliasesJSON), string(candidatesJSON))
	default:
		_, _ = fmt.Fprintf(&prompt, `You are a strict IRC trivia judge.
Topic: %s
Question: %s
Official answer: %s
Official normalized answer: %s
Accepted aliases (JSON): %s

Candidate guesses (chronological, JSON):
%s

Decide if any candidate guess is clearly equivalent to the official answer.
Strict judging rules:
- Accept only if the guess is in the same factual ballpark and clearly refers to the same answer.
- Reject partial, vague, broad, related-but-not-equivalent, or incorrect guesses.
- Minor spelling/wording variation is okay only when meaning is clearly the same.
- If multiple candidates qualify, pick the earliest one (lowest elapsed_ms).
- If none qualify, approved must be false.

Return ONLY JSON with this exact shape:
{
  "approved": true or false,
  "guess_id": integer (0 if approved is false),
  "confidence": number between 0 and 1,
  "reason": "short explanation"
}
`, strings.TrimSpace(req.Topic), strings.TrimSpace(req.Question), strings.TrimSpace(req.Answer), NormalizeAnswer(req.Answer), string(aliasesJSON), string(candidatesJSON))
	}

	return prompt.String(), nil
}

func difficultyPromptGuidance(difficulty string) string {
	switch NormalizeDifficulty(difficulty) {
	case DifficultyEasy:
		return "- easy: broad/common knowledge; avoid niche version-specific facts; keep wording very straightforward."
	case DifficultyHard:
		return "- hard: require deeper subject knowledge with precise facts, but keep answer short and objective."
	default:
		return "- medium: balanced difficulty; not trivial, not obscure; typical knowledgeable user should solve with some thought."
	}
}

func normalizeReasoningEffort(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "none", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(input))
	default:
		return "medium"
	}
}

func validateJudgeDecision(decision *JudgeDecision, candidates []JudgeGuessCandidate) error {
	if decision == nil {
		return fmt.Errorf("invalid trivia judge payload: empty decision")
	}

	if decision.Confidence < 0 {
		decision.Confidence = 0
	}
	if decision.Confidence > 1 {
		decision.Confidence = 1
	}
	decision.Reason = strings.TrimSpace(decision.Reason)

	if !decision.Approved {
		decision.GuessID = 0
		return nil
	}

	if decision.GuessID <= 0 {
		return fmt.Errorf("invalid trivia judge payload: approved=true but guess_id missing")
	}

	for _, candidate := range candidates {
		if candidate.ID == decision.GuessID {
			return nil
		}
	}

	return fmt.Errorf("invalid trivia judge payload: guess_id %d not in candidate set", decision.GuessID)
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
	if len(question.Answer) > maxAnswerLength {
		return fmt.Errorf("invalid trivia payload: answer exceeds %d chars", maxAnswerLength)
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
		if len(trimmed) > maxAliasLength {
			continue
		}
		validAliases = append(validAliases, trimmed)
	}
	question.Aliases = validAliases

	return nil
}

func validateGeneratedCodeQuestion(question *GeneratedCodeQuestion, expectedLanguage string) error {
	if question == nil {
		return fmt.Errorf("invalid code payload: empty")
	}

	expectedLanguage = strings.TrimSpace(expectedLanguage)
	question.Language = strings.TrimSpace(question.Language)
	question.Question = strings.TrimSpace(question.Question)
	question.Answer = strings.TrimSpace(question.Answer)
	question.Hint = strings.TrimSpace(question.Hint)
	question.UniquenessKey = strings.TrimSpace(question.UniquenessKey)
	question.ValidatorType = strings.TrimSpace(question.ValidatorType)

	if question.Language == "" {
		return fmt.Errorf("invalid code payload: language is empty")
	}
	if len(question.Language) > maxCodeLanguageField {
		return fmt.Errorf("invalid code payload: language too long")
	}

	canonicalResponseLanguage, ok := NormalizeCodeLanguage(question.Language)
	if !ok || canonicalResponseLanguage != expectedLanguage {
		return fmt.Errorf("invalid code payload: language mismatch (got %s expected %s)", question.Language, expectedLanguage)
	}
	question.Language = canonicalResponseLanguage

	if question.Question == "" {
		return fmt.Errorf("invalid code payload: question is empty")
	}
	if strings.ContainsAny(question.Question, "\n\r") {
		return fmt.Errorf("invalid code payload: question must be one line")
	}
	if len(question.Question) > maxCodeQuestionLen {
		return fmt.Errorf("invalid code payload: question exceeds %d chars", maxCodeQuestionLen)
	}
	if question.Answer == "" {
		return fmt.Errorf("invalid code payload: answer is empty")
	}
	if len(question.Answer) > maxCodeAnswerLength {
		return fmt.Errorf("invalid code payload: answer exceeds %d chars", maxCodeAnswerLength)
	}
	if strings.ContainsAny(question.Answer, "\n\r") {
		return fmt.Errorf("invalid code payload: answer must be one line")
	}
	if strings.HasPrefix(question.Answer, "```") || strings.HasSuffix(question.Answer, "```") {
		return fmt.Errorf("invalid code payload: fenced code is not allowed")
	}
	if NormalizeCodeAnswer(question.Answer) == "" {
		return fmt.Errorf("invalid code payload: answer failed code normalization")
	}
	if question.Hint == "" {
		return fmt.Errorf("invalid code payload: hint is empty")
	}
	if len(question.Hint) > maxCodeHintLength {
		return fmt.Errorf("invalid code payload: hint exceeds %d chars", maxCodeHintLength)
	}
	if question.UniquenessKey == "" {
		return fmt.Errorf("invalid code payload: uniqueness_key is empty")
	}
	if strings.ContainsAny(question.UniquenessKey, "\n\r") {
		return fmt.Errorf("invalid code payload: uniqueness_key must be one line")
	}
	if len(question.UniquenessKey) > maxCodeUniqueKeyLen {
		return fmt.Errorf("invalid code payload: uniqueness_key exceeds %d chars", maxCodeUniqueKeyLen)
	}
	if strings.ToLower(question.ValidatorType) != ValidatorNormalizedExact {
		return fmt.Errorf("invalid code payload: validator_type must be %s", ValidatorNormalizedExact)
	}
	question.ValidatorType = ValidatorNormalizedExact

	validAliases := make([]string, 0, len(question.Aliases))
	for _, alias := range question.Aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > maxCodeAliasLength {
			continue
		}
		if strings.ContainsAny(trimmed, "\n\r") {
			continue
		}
		if NormalizeCodeAnswer(trimmed) == "" {
			continue
		}
		validAliases = append(validAliases, trimmed)
	}
	question.Aliases = validAliases

	return nil
}

func countWords(text string) int {
	return len(strings.Fields(strings.TrimSpace(text)))
}
