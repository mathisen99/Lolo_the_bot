package trivia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
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
	defaultMaxOutputTokens    = 420
	maxOutputTokensRetryStep  = 80
	maxOutputTokensCeiling    = 1400
	triviaGenerationMinTokens = 420
	codeGenerationMinTokens   = 1100
	judgeGenerationMinTokens  = 300
	maxAnswerLength           = 160
	maxAliasLength            = 160
	maxCodeQuestionLen        = 220
	maxCodeAnswerLength       = 280
	maxCodeAliasLength        = 280
	maxCodeHintLength         = 200
	maxCodeUniqueKeyLen       = 180
	maxCodeLanguageField      = 32
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

	maxOutputTokens := resolveMaxOutputTokens(g.config.MaxOutputTokens, attempt, triviaGenerationMinTokens)

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

// GenerateCodeQuestion creates a one-line coding question for a specific language and difficulty.
func (g *Generator) GenerateCodeQuestion(ctx context.Context, language, difficulty string, attempt int, avoidKeys, avoidQuestions []string) (*GeneratedCodeQuestion, error) {
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

	maxOutputTokens := resolveMaxOutputTokens(g.config.MaxOutputTokens, attempt, codeGenerationMinTokens)

	difficulty = NormalizeDifficulty(difficulty)
	prompt := buildCodePrompt(canonicalLanguage, difficulty, attempt, avoidKeys, avoidQuestions)

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

// JudgeClosestGuess runs post-timeout close-answer judging for trivia/code rounds.
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
		"max_output_tokens": resolveMaxOutputTokens(g.config.MaxOutputTokens, 1, judgeGenerationMinTokens),
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

func buildCodePrompt(language, difficulty string, attempt int, avoidKeys, avoidQuestions []string) string {
	language = strings.TrimSpace(language)
	difficulty = NormalizeDifficulty(difficulty)
	now := time.Now().UTC().Format("2006-01-02")

	var prompt strings.Builder
	_, _ = fmt.Fprintf(&prompt, `Generate a one-line coding quiz for IRC.

Language: %s
Difficulty: %s
Current date (UTC): %s

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
- Use modern, up-to-date syntax and standard-library usage as of the current date
- Avoid deprecated or legacy-only constructs unless explicitly required

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
`, language, difficulty, now, language, language)

	_, _ = fmt.Fprintf(&prompt, "\nDifficulty requirements (%s):\n%s\n", difficulty, codeDifficultyPromptGuidance(difficulty))

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

func codeDifficultyPromptGuidance(difficulty string) string {
	switch NormalizeDifficulty(difficulty) {
	case DifficultyEasy:
		return "- easy: very common syntax/stdlib one-liners; avoid tricky edge cases."
	case DifficultyHard:
		return "- hard: still one-line solvable, but require deeper language knowledge or precise syntax."
	default:
		return "- medium: balanced one-liners; not trivial but not obscure."
	}
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
		_, _ = fmt.Fprintf(&prompt, `You are a strict-but-fair IRC trivia judge.
Topic: %s
Question: %s
Official answer: %s
Official normalized answer: %s
Accepted aliases (JSON): %s

Candidate guesses (chronological, JSON):
%s

Decide if any candidate guess is clearly equivalent to the official answer.
Strict judging rules:
- Accept if the guess clearly identifies the same specific answer in this question's context.
- Accept minor spelling/wording variation, abbreviations, and concise shorthand when meaning is unambiguous.
- Reject guesses that are wrong, too broad, ambiguous for this question, or only loosely related.
- When in doubt, reject.
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

func resolveMaxOutputTokens(configValue, attempt, minimum int) int {
	maxOutputTokens := configValue
	if maxOutputTokens <= 0 {
		maxOutputTokens = defaultMaxOutputTokens
	}
	if maxOutputTokens < minimum {
		maxOutputTokens = minimum
	}
	if attempt < 1 {
		attempt = 1
	}
	maxOutputTokens += (attempt - 1) * maxOutputTokensRetryStep
	if maxOutputTokens > maxOutputTokensCeiling {
		maxOutputTokens = maxOutputTokensCeiling
	}
	return maxOutputTokens
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
	text, status, incompleteReason, err := extractResponseText(rawResponse)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("openai response did not include output text%s", responseMetadataSuffix(status, incompleteReason))
	}

	jsonText, ok := extractFirstJSONObject(text)
	if !ok {
		var asJSONString string
		if err := json.Unmarshal([]byte(text), &asJSONString); err == nil {
			jsonText, ok = extractFirstJSONObject(asJSONString)
		}
	}
	if !ok {
		if kvJSON, kvOK := parseLooseKeyValueJSONObject(text); kvOK {
			jsonText = kvJSON
			ok = true
		}
	}
	if !ok {
		preview := summarizeForLog(text, 220)
		if preview != "" {
			return "", fmt.Errorf("openai response did not include a valid JSON object%s; preview=%q", responseMetadataSuffix(status, incompleteReason), preview)
		}
		return "", fmt.Errorf("openai response did not include a valid JSON object%s", responseMetadataSuffix(status, incompleteReason))
	}
	return jsonText, nil
}

func extractResponseText(rawResponse []byte) (string, string, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(rawResponse, &payload); err != nil {
		return "", "", "", fmt.Errorf("failed to parse openai response envelope: %w", err)
	}

	status := strings.TrimSpace(flattenTextCandidate(payload["status"]))
	incompleteReason := ""
	if details, ok := payload["incomplete_details"].(map[string]any); ok {
		incompleteReason = strings.TrimSpace(flattenTextCandidate(details["reason"]))
	}

	fragments := make([]string, 0, 8)
	appendTextCandidate(&fragments, flattenTextCandidate(payload["output_text"]))
	appendJSONCandidate(&fragments, payload["output_parsed"])

	outputItems, _ := payload["output"].([]any)
	for _, item := range outputItems {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		appendTextCandidate(&fragments, flattenTextCandidate(itemMap["output_text"]))
		appendTextCandidate(&fragments, flattenTextCandidate(itemMap["text"]))
		appendTextCandidate(&fragments, flattenTextCandidate(itemMap["arguments"]))
		appendJSONCandidate(&fragments, itemMap["json"])

		contentItems, _ := itemMap["content"].([]any)
		for _, content := range contentItems {
			contentMap, ok := content.(map[string]any)
			if !ok {
				continue
			}
			appendTextCandidate(&fragments, flattenTextCandidate(contentMap["output_text"]))
			appendTextCandidate(&fragments, flattenTextCandidate(contentMap["text"]))
			appendTextCandidate(&fragments, flattenTextCandidate(contentMap["arguments"]))
			appendTextCandidate(&fragments, flattenTextCandidate(contentMap["value"]))
			appendJSONCandidate(&fragments, contentMap["json"])
		}
	}

	return strings.TrimSpace(strings.Join(fragments, "\n")), status, incompleteReason, nil
}

func appendTextCandidate(fragments *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	*fragments = append(*fragments, text)
}

func appendJSONCandidate(fragments *[]string, value any) {
	if value == nil {
		return
	}

	if text, ok := value.(string); ok {
		appendTextCandidate(fragments, text)
		return
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	appendTextCandidate(fragments, string(encoded))
}

func flattenTextCandidate(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			part := flattenTextCandidate(item)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case map[string]any:
		if part := flattenTextCandidate(v["value"]); part != "" {
			return part
		}
		if part := flattenTextCandidate(v["text"]); part != "" {
			return part
		}
	}
	return ""
}

func responseMetadataSuffix(status, reason string) string {
	parts := make([]string, 0, 2)
	status = strings.TrimSpace(status)
	reason = strings.TrimSpace(reason)

	if status != "" {
		parts = append(parts, "status="+status)
	}
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}

	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func summarizeForLog(text string, maxLen int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if clean == "" {
		return ""
	}
	if maxLen <= 0 || len(clean) <= maxLen {
		return clean
	}
	if maxLen <= 3 {
		return clean[:maxLen]
	}
	return clean[:maxLen-3] + "..."
}

func parseLooseKeyValueJSONObject(text string) (string, bool) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return "", false
	}

	// Strip optional markdown fences first.
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```JSON")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	lines := strings.Split(cleaned, "\n")
	fields := make(map[string]string, 10)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "-*"))
		}

		sep := strings.Index(trimmed, ":")
		if sep <= 0 {
			continue
		}

		key := normalizeLooseKVKey(trimmed[:sep])
		value := strings.TrimSpace(trimmed[sep+1:])
		value = strings.Trim(value, `"'`)
		if key == "" || value == "" {
			continue
		}
		fields[key] = value
	}

	if len(fields) == 0 {
		return "", false
	}

	if judgeJSON, ok := buildLooseJudgeJSON(fields); ok {
		return judgeJSON, true
	}
	_, hasLanguage := fields["language"]
	_, hasValidator := fields["validator_type"]
	if hasLanguage || hasValidator {
		if codeJSON, ok := buildLooseCodeJSON(fields); ok {
			return codeJSON, true
		}
	}
	if triviaJSON, ok := buildLooseTriviaJSON(fields); ok {
		return triviaJSON, true
	}
	if !hasLanguage && !hasValidator {
		if codeJSON, ok := buildLooseCodeJSON(fields); ok {
			return codeJSON, true
		}
	}

	return "", false
}

func normalizeLooseKVKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")

	switch normalized {
	case "q":
		return "question"
	case "a":
		return "answer"
	case "unique_key", "uniqueness", "uniq_key":
		return "uniqueness_key"
	case "lang":
		return "language"
	case "validator":
		return "validator_type"
	}

	return normalized
}

func buildLooseJudgeJSON(fields map[string]string) (string, bool) {
	approvedRaw, hasApproved := fields["approved"]
	if !hasApproved {
		return "", false
	}

	approved, ok := parseLooseBool(approvedRaw)
	if !ok {
		return "", false
	}

	guessID := int64(0)
	if value, exists := fields["guess_id"]; exists {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed >= 0 {
			guessID = parsed
		}
	}
	if !approved {
		guessID = 0
	}

	confidence := 0.0
	if value, exists := fields["confidence"]; exists {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			confidence = math.Max(0, math.Min(1, parsed))
		}
	}

	reason := strings.TrimSpace(fields["reason"])

	payload := map[string]any{
		"approved":   approved,
		"guess_id":   guessID,
		"confidence": confidence,
		"reason":     reason,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func buildLooseTriviaJSON(fields map[string]string) (string, bool) {
	question := strings.TrimSpace(fields["question"])
	answer := strings.TrimSpace(fields["answer"])
	hint := strings.TrimSpace(fields["hint"])
	if question == "" || answer == "" || hint == "" {
		return "", false
	}

	aliases := parseLooseAliases(fields["aliases"])
	uniquenessKey := strings.TrimSpace(fields["uniqueness_key"])
	if uniquenessKey == "" {
		uniquenessKey = strings.TrimSpace(fields["unique_key"])
	}

	payload := map[string]any{
		"question":       question,
		"answer":         answer,
		"aliases":        aliases,
		"hint":           hint,
		"uniqueness_key": uniquenessKey,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func buildLooseCodeJSON(fields map[string]string) (string, bool) {
	language := strings.TrimSpace(fields["language"])
	question := strings.TrimSpace(fields["question"])
	answer := strings.TrimSpace(fields["answer"])
	hint := strings.TrimSpace(fields["hint"])
	if language == "" || question == "" || answer == "" || hint == "" {
		return "", false
	}

	validatorType := strings.TrimSpace(fields["validator_type"])
	if validatorType == "" {
		validatorType = ValidatorNormalizedExact
	}

	aliases := parseLooseAliases(fields["aliases"])
	uniquenessKey := strings.TrimSpace(fields["uniqueness_key"])
	if uniquenessKey == "" {
		uniquenessKey = strings.TrimSpace(fields["unique_key"])
	}

	payload := map[string]any{
		"language":       language,
		"question":       question,
		"answer":         answer,
		"aliases":        aliases,
		"hint":           hint,
		"uniqueness_key": uniquenessKey,
		"validator_type": validatorType,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func parseLooseAliases(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	var aliases []string
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		if err := json.Unmarshal([]byte(raw), &aliases); err == nil {
			clean := make([]string, 0, len(aliases))
			for _, alias := range aliases {
				trimmed := strings.TrimSpace(alias)
				if trimmed != "" {
					clean = append(clean, trimmed)
				}
			}
			return clean
		}
	}

	parts := strings.Split(raw, ",")
	aliases = make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), `"'`)
		if trimmed == "" {
			continue
		}
		aliases = append(aliases, trimmed)
	}
	return aliases
}

func parseLooseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1":
		return true, true
	case "false", "no", "n", "0":
		return false, true
	default:
		return false, false
	}
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
