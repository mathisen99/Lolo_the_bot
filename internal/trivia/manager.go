package trivia

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

var ignoredSimilarityTokens = map[string]struct{}{
	"what":  {},
	"which": {},
	"who":   {},
	"whom":  {},
	"whose": {},
	"when":  {},
	"where": {},
	"why":   {},
	"how":   {},
	"is":    {},
	"are":   {},
	"was":   {},
	"were":  {},
	"do":    {},
	"does":  {},
	"did":   {},
}

const (
	longAnswerJudgeWordThreshold = 3
	judgeConfidenceThreshold     = 0.84
	maxJudgeCandidates           = 120
)

type activeRound struct {
	RoundID           int64
	Mode              string
	Channel           string
	Topic             string
	Language          string
	QuestionID        int64
	ValidatorType     string
	Question          string
	Answer            string
	Aliases           []string
	Hint              string
	StartedAt         time.Time
	AcceptedAnswers   map[string]struct{}
	Settings          ChannelSettings
	HintUsed          bool
	Guesses           []GuessLog
	NextGuessID       int
	closed            bool
	timeoutTimer      *time.Timer
	NormalizedAnswer  string
	NormalizedAliases []string
}

// ManagerConfig controls trivia runtime behavior.
type ManagerConfig struct {
	Store             *Store
	Generator         *Generator
	Logger            output.Logger
	GenerationRetries int
}

// Manager coordinates active rounds, generation, and persistence.
type Manager struct {
	store             *Store
	generator         *Generator
	logger            output.Logger
	generationRetries int

	mu            sync.Mutex
	activeRounds  map[string]*activeRound
	startingRound map[string]bool
	sendMessage   func(target, message string) error
}

// NewManager creates a trivia manager.
func NewManager(config ManagerConfig) *Manager {
	retries := config.GenerationRetries
	if retries <= 0 {
		retries = 5
	}

	return &Manager{
		store:             config.Store,
		generator:         config.Generator,
		logger:            config.Logger,
		generationRetries: retries,
		activeRounds:      make(map[string]*activeRound),
		startingRound:     make(map[string]bool),
	}
}

// SetSendMessageFunc sets callback used for async timeout announcements.
func (m *Manager) SetSendMessageFunc(fn func(target, message string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendMessage = fn
}

// Shutdown stops all active timers and marks active rounds as cancelled.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	rounds := make([]*activeRound, 0, len(m.activeRounds))
	for _, round := range m.activeRounds {
		rounds = append(rounds, round)
	}
	m.activeRounds = make(map[string]*activeRound)
	m.startingRound = make(map[string]bool)
	m.mu.Unlock()

	for _, round := range rounds {
		if round.timeoutTimer != nil {
			round.timeoutTimer.Stop()
		}
		if err := m.store.FinalizeRoundNoWinner(round.RoundID, round.HintUsed, "cancelled", time.Now()); err != nil {
			m.logger.Warning("Failed to mark trivia round cancelled on shutdown (channel=%s): %v", round.Channel, err)
		}
	}
}

// StartRound creates and announces a new channel round.
func (m *Manager) StartRound(ctx context.Context, channel, topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", ErrTopicRequired
	}

	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return "", err
	}
	if !settings.Enabled {
		return "", ErrTriviaDisabled
	}

	m.mu.Lock()
	if m.activeRounds[channel] != nil || m.startingRound[channel] {
		m.mu.Unlock()
		return "", ErrRoundAlreadyActive
	}
	m.startingRound[channel] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.startingRound, channel)
		m.mu.Unlock()
	}()

	question, err := m.generateAndPersistQuestion(ctx, topic, settings.Difficulty)
	if err != nil {
		return "", err
	}

	return m.startRoundFromStoredQuestion(channel, settings, ModeTrivia, topic, "", question)
}

// StartCodeRound creates and announces a new channel code round.
func (m *Manager) StartCodeRound(ctx context.Context, channel, language string) (string, error) {
	canonicalLanguage, ok := NormalizeCodeLanguage(language)
	if !ok {
		return "", ErrUnsupportedCodeLanguage
	}

	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return "", err
	}
	if !settings.Enabled {
		return "", ErrTriviaDisabled
	}

	m.mu.Lock()
	if m.activeRounds[channel] != nil || m.startingRound[channel] {
		m.mu.Unlock()
		return "", ErrRoundAlreadyActive
	}
	m.startingRound[channel] = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.startingRound, channel)
		m.mu.Unlock()
	}()

	question, err := m.generateAndPersistCodeQuestion(ctx, canonicalLanguage, settings.CodeDifficulty)
	if err != nil {
		return "", err
	}

	return m.startRoundFromStoredQuestion(channel, settings, ModeCode, canonicalLanguage, canonicalLanguage, question)
}

func (m *Manager) startRoundFromStoredQuestion(channel string, settings ChannelSettings, mode, topic, language string, question *StoredQuestion) (string, error) {
	accepted, normalizedAnswer, normalizedAliases := buildAcceptedAnswerSet(mode, question.Answer, question.Aliases)
	if len(accepted) == 0 || normalizedAnswer == "" {
		return "", ErrGenerationFailed
	}

	roundAnswerTimeSeconds := settings.AnswerTimeSeconds
	roundDifficulty := NormalizeDifficulty(settings.Difficulty)
	if NormalizeMode(mode) == ModeCode {
		if settings.CodeAnswerTimeSeconds > 0 {
			roundAnswerTimeSeconds = settings.CodeAnswerTimeSeconds
		}
		roundDifficulty = NormalizeDifficulty(settings.CodeDifficulty)
	}
	if roundAnswerTimeSeconds <= 0 {
		roundAnswerTimeSeconds = 30
	}

	roundSettings := settings
	roundSettings.AnswerTimeSeconds = roundAnswerTimeSeconds
	roundSettings.Difficulty = roundDifficulty

	startedAt := time.Now()
	roundID, err := m.store.StartRound(channel, topic, mode, language, question.ID, startedAt)
	if err != nil {
		return "", err
	}

	round := &activeRound{
		RoundID:           roundID,
		Mode:              mode,
		Channel:           channel,
		Topic:             topic,
		Language:          language,
		QuestionID:        question.ID,
		ValidatorType:     normalizeValidatorType(question.ValidatorType),
		Question:          question.Question,
		Answer:            question.Answer,
		Aliases:           append([]string(nil), question.Aliases...),
		Hint:              question.Hint,
		StartedAt:         startedAt,
		AcceptedAnswers:   accepted,
		Settings:          roundSettings,
		HintUsed:          false,
		Guesses:           make([]GuessLog, 0, 24),
		NextGuessID:       1,
		closed:            false,
		NormalizedAnswer:  normalizedAnswer,
		NormalizedAliases: normalizedAliases,
	}

	duration := time.Duration(roundAnswerTimeSeconds) * time.Second
	round.timeoutTimer = time.AfterFunc(duration, func() {
		m.handleTimeout(channel, roundID)
	})

	m.mu.Lock()
	if m.activeRounds[channel] != nil {
		m.mu.Unlock()
		round.timeoutTimer.Stop()
		_ = m.store.FinalizeRoundNoWinner(roundID, false, "cancelled", time.Now())
		return "", ErrRoundAlreadyActive
	}
	m.activeRounds[channel] = round
	m.mu.Unlock()

	switch mode {
	case ModeCode:
		m.logger.Info("Code round started: channel=%s language=%s difficulty=%s question_id=%d round_id=%d", channel, language, roundDifficulty, question.ID, roundID)
		return fmt.Sprintf(
			"Code (%s, %s): %s | You have %ds. Answer with one line of code in normal channel text. Use !hint for a hint.",
			language,
			roundDifficulty,
			question.Question,
			roundAnswerTimeSeconds,
		), nil
	default:
		m.logger.Info("Trivia round started: channel=%s topic=%s difficulty=%s question_id=%d round_id=%d", channel, topic, roundDifficulty, question.ID, roundID)
		return fmt.Sprintf(
			"Trivia (%s): %s | You have %ds. Answer with normal channel text. Use !hint for a hint.",
			topic,
			question.Question,
			roundAnswerTimeSeconds,
		), nil
	}
}

// TryAnswer checks a normal channel message against active round answers.
// Returns response text and handled=true only when the message wins the round.
func (m *Manager) TryAnswer(channel, nick, message string) (string, bool, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", false, nil
	}

	m.mu.Lock()
	round := m.activeRounds[channel]
	if round == nil || round.closed {
		m.mu.Unlock()
		return "", false, nil
	}

	round.Guesses = append(round.Guesses, GuessLog{
		ID:        round.NextGuessID,
		Nick:      nick,
		Message:   message,
		Timestamp: time.Now(),
	})
	round.NextGuessID++

	if !isCorrectAnswer(round, message) {
		m.mu.Unlock()
		return "", false, nil
	}

	round.closed = true
	delete(m.activeRounds, channel)
	if round.timeoutTimer != nil {
		round.timeoutTimer.Stop()
	}
	m.mu.Unlock()

	elapsed := time.Since(round.StartedAt)
	points := calculatePoints(round.Settings, elapsed, round.HintUsed)
	endedAt := time.Now()

	updatedScore, err := m.store.FinalizeRoundWin(
		round.RoundID,
		channel,
		nick,
		message,
		points,
		round.HintUsed,
		endedAt,
	)
	if err != nil {
		return "", false, err
	}

	m.logger.Info("Game winner: mode=%s channel=%s round_id=%d winner=%s points=%d", round.Mode, channel, round.RoundID, nick, points)

	responsePrefix := "Answer"
	if round.Mode == ModeCode {
		responsePrefix = "Code"
	}

	response := fmt.Sprintf("%s got it! %s: %s (+%d points, total: %d).", nick, responsePrefix, round.Answer, points, updatedScore)
	return response, true, nil
}

// UseHint reveals a hint for the active channel round.
func (m *Manager) UseHint(channel string) (string, error) {
	m.mu.Lock()
	round := m.activeRounds[channel]
	if round == nil || round.closed {
		m.mu.Unlock()
		return "", ErrNoActiveRound
	}
	if !round.Settings.HintsEnabled {
		m.mu.Unlock()
		return "", ErrHintsDisabled
	}
	if round.HintUsed {
		m.mu.Unlock()
		return "", ErrHintAlreadyUsed
	}
	round.HintUsed = true
	penalty := round.Settings.HintPenalty
	hint := round.Hint
	m.mu.Unlock()

	m.logger.Info("Trivia hint used: channel=%s penalty=%d", channel, penalty)
	return fmt.Sprintf("Hint: %s (hint penalty: -%d points).", hint, penalty), nil
}

// HasActiveRound reports whether a channel currently has an active or starting round.
func (m *Manager) HasActiveRound(channel string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeRounds[channel] != nil || m.startingRound[channel]
}

// GetSettings returns the channel trivia settings.
func (m *Manager) GetSettings(channel string) (ChannelSettings, error) {
	return m.store.GetSettings(channel)
}

// UpdateAnswerTime updates answer timeout seconds for a channel.
func (m *Manager) UpdateAnswerTime(channel string, seconds int) (ChannelSettings, error) {
	if seconds < 5 || seconds > 600 {
		return ChannelSettings{}, fmt.Errorf("answer time must be between 5 and 600 seconds")
	}
	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.AnswerTimeSeconds = seconds
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdateCodeAnswerTime updates code answer timeout seconds for a channel.
func (m *Manager) UpdateCodeAnswerTime(channel string, seconds int) (ChannelSettings, error) {
	if seconds < 5 || seconds > 600 {
		return ChannelSettings{}, fmt.Errorf("code answer time must be between 5 and 600 seconds")
	}
	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.CodeAnswerTimeSeconds = seconds
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdateHintsEnabled updates hint usage behavior for a channel.
func (m *Manager) UpdateHintsEnabled(channel string, enabled bool) (ChannelSettings, error) {
	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.HintsEnabled = enabled
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdatePoints updates point-related settings for a channel.
func (m *Manager) UpdatePoints(channel string, base, minimum, hintPenalty int) (ChannelSettings, error) {
	if base <= 0 {
		return ChannelSettings{}, fmt.Errorf("base points must be positive")
	}
	if minimum < 0 {
		return ChannelSettings{}, fmt.Errorf("minimum points must be non-negative")
	}
	if minimum > base {
		return ChannelSettings{}, fmt.Errorf("minimum points cannot exceed base points")
	}
	if hintPenalty < 0 {
		return ChannelSettings{}, fmt.Errorf("hint penalty must be non-negative")
	}

	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.BasePoints = base
	settings.MinimumPoints = minimum
	settings.HintPenalty = hintPenalty
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdateEnabled toggles trivia for a channel.
func (m *Manager) UpdateEnabled(channel string, enabled bool) (ChannelSettings, error) {
	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.Enabled = enabled
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdateDifficulty updates question difficulty for a channel.
func (m *Manager) UpdateDifficulty(channel, difficulty string) (ChannelSettings, error) {
	if !IsValidDifficulty(difficulty) {
		return ChannelSettings{}, fmt.Errorf("difficulty must be easy, medium, or hard")
	}

	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.Difficulty = NormalizeDifficulty(difficulty)
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// UpdateCodeDifficulty updates code question difficulty for a channel.
func (m *Manager) UpdateCodeDifficulty(channel, difficulty string) (ChannelSettings, error) {
	if !IsValidDifficulty(difficulty) {
		return ChannelSettings{}, fmt.Errorf("code difficulty must be easy, medium, or hard")
	}

	settings, err := m.store.GetSettings(channel)
	if err != nil {
		return ChannelSettings{}, err
	}
	settings.CodeDifficulty = NormalizeDifficulty(difficulty)
	if err := m.store.SaveSettings(channel, settings); err != nil {
		return ChannelSettings{}, err
	}
	return settings, nil
}

// GetTopScores returns top leaderboard entries.
func (m *Manager) GetTopScores(channel string, limit int) ([]ScoreEntry, error) {
	return m.store.GetTopScores(channel, limit)
}

// GetScore returns a score for nick in channel.
func (m *Manager) GetScore(channel, nick string) (int, bool, error) {
	return m.store.GetScore(channel, nick)
}

// SetScore sets a user's score.
func (m *Manager) SetScore(channel, nick string, points int) error {
	return m.store.SetScore(channel, nick, points)
}

// AddScore modifies score by delta and returns the new score.
func (m *Manager) AddScore(channel, nick string, delta int) (int, error) {
	return m.store.AddScore(channel, nick, delta)
}

// ResetScore removes a user's score row.
func (m *Manager) ResetScore(channel, nick string) error {
	return m.store.ResetScore(channel, nick)
}

func (m *Manager) generateAndPersistQuestion(ctx context.Context, topic, difficulty string) (*StoredQuestion, error) {
	if m.generator == nil {
		return nil, ErrGeneratorDisabled
	}

	recentTopicQuestions, err := m.store.GetRecentQuestionsByTopic(topic, 50)
	if err != nil {
		return nil, err
	}
	recentQuestionTexts := make([]string, 0, len(recentTopicQuestions))
	for _, q := range recentTopicQuestions {
		if strings.TrimSpace(q.Question) == "" {
			continue
		}
		recentQuestionTexts = append(recentQuestionTexts, q.Question)
	}

	var lastErr error
	rejectedKeys := make([]string, 0, m.generationRetries)
	rejectedQuestions := make([]string, 0, m.generationRetries)
	for attempt := 1; attempt <= m.generationRetries; attempt++ {
		avoidQuestions := make([]string, 0, len(recentQuestionTexts)+len(rejectedQuestions))
		avoidQuestions = append(avoidQuestions, recentQuestionTexts...)
		avoidQuestions = append(avoidQuestions, rejectedQuestions...)

		generated, err := m.generator.GenerateQuestion(ctx, topic, difficulty, attempt, rejectedKeys, avoidQuestions)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if errors.Is(err, errMissingAPIKey) || errors.Is(err, ErrGeneratorDisabled) {
				m.logger.Warning("Trivia generator unavailable: %v", err)
				return nil, ErrGeneratorDisabled
			}
			m.logger.Warning("Trivia generation attempt %d/%d failed: %v", attempt, m.generationRetries, err)
			continue
		}

		normalizedUnique := NormalizeDedupKey(generated.UniquenessKey)
		if normalizedUnique == "" {
			normalizedUnique = NormalizeDedupKey(generated.Question)
		}
		normalizedQuestion := NormalizeDedupKey(generated.Question)
		if normalizedUnique == "" || normalizedQuestion == "" {
			lastErr = fmt.Errorf("generated trivia had empty normalized dedup keys")
			m.logger.Warning("Trivia generation attempt %d/%d produced invalid dedup keys", attempt, m.generationRetries)
			continue
		}

		uniqueHash := HashNormalized(normalizedUnique)
		questionHash := HashNormalized(normalizedQuestion)

		duplicate, err := m.store.IsQuestionDuplicate(uniqueHash, questionHash)
		if err != nil {
			return nil, err
		}
		if duplicate {
			m.logger.Info("Rejected duplicate trivia question (attempt %d/%d, topic=%s)", attempt, m.generationRetries, topic)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
			rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
			lastErr = ErrGenerationFailed
			continue
		}

		nearDuplicate, nearReason, err := m.isNearDuplicateQuestion(topic, generated.Question, generated.Answer)
		if err != nil {
			return nil, err
		}
		if nearDuplicate {
			m.logger.Info("Rejected near-duplicate trivia question (attempt %d/%d, topic=%s, reason=%s)", attempt, m.generationRetries, topic, nearReason)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
			rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
			lastErr = ErrGenerationFailed
			continue
		}

		stored := &StoredQuestion{
			Mode:           ModeTrivia,
			Topic:          topic,
			Language:       "",
			Question:       generated.Question,
			Answer:         generated.Answer,
			Aliases:        generated.Aliases,
			Hint:           generated.Hint,
			ValidatorType:  ValidatorNormalizedExact,
			UniquenessKey:  normalizedUnique,
			UniquenessHash: uniqueHash,
			QuestionHash:   questionHash,
			CreatedAt:      time.Now(),
		}

		id, err := m.store.InsertQuestion(stored)
		if err != nil {
			lastErr = err
			if isUniqueConstraintErr(err) {
				m.logger.Info("Rejected duplicate trivia question from unique constraint (attempt %d/%d)", attempt, m.generationRetries)
				rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
				rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
				rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
				continue
			}
			return nil, err
		}

		stored.ID = id
		return stored, nil
	}

	if lastErr != nil {
		m.logger.Warning("Trivia generation exhausted retries: %v", lastErr)
	}
	return nil, ErrGenerationFailed
}

func (m *Manager) generateAndPersistCodeQuestion(ctx context.Context, language, difficulty string) (*StoredQuestion, error) {
	if m.generator == nil {
		return nil, ErrGeneratorDisabled
	}
	difficulty = NormalizeDifficulty(difficulty)

	recentLanguageQuestions, err := m.store.GetRecentCodeQuestionsByLanguage(language, 50)
	if err != nil {
		return nil, err
	}

	recentQuestionTexts := make([]string, 0, len(recentLanguageQuestions))
	for _, q := range recentLanguageQuestions {
		if strings.TrimSpace(q.Question) == "" {
			continue
		}
		recentQuestionTexts = append(recentQuestionTexts, q.Question)
	}

	var lastErr error
	rejectedKeys := make([]string, 0, m.generationRetries)
	rejectedQuestions := make([]string, 0, m.generationRetries)

	for attempt := 1; attempt <= m.generationRetries; attempt++ {
		avoidQuestions := make([]string, 0, len(recentQuestionTexts)+len(rejectedQuestions))
		avoidQuestions = append(avoidQuestions, recentQuestionTexts...)
		avoidQuestions = append(avoidQuestions, rejectedQuestions...)

		generated, err := m.generator.GenerateCodeQuestion(ctx, language, difficulty, attempt, rejectedKeys, avoidQuestions)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if errors.Is(err, errMissingAPIKey) || errors.Is(err, ErrGeneratorDisabled) {
				m.logger.Warning("Code generator unavailable: %v", err)
				return nil, ErrGeneratorDisabled
			}
			m.logger.Warning("Code generation attempt %d/%d failed (language=%s difficulty=%s): %v", attempt, m.generationRetries, language, difficulty, err)
			continue
		}

		normalizedUnique := NormalizeDedupKey(fmt.Sprintf("code %s %s", language, generated.UniquenessKey))
		if normalizedUnique == "" {
			normalizedUnique = NormalizeDedupKey(fmt.Sprintf("code %s %s", language, generated.Question))
		}
		normalizedQuestion := NormalizeDedupKey(fmt.Sprintf("code %s %s", language, generated.Question))
		if normalizedUnique == "" || normalizedQuestion == "" {
			lastErr = fmt.Errorf("generated code quiz had empty normalized dedup keys")
			m.logger.Warning("Code generation attempt %d/%d produced invalid dedup keys (language=%s difficulty=%s)", attempt, m.generationRetries, language, difficulty)
			continue
		}

		uniqueHash := HashNormalized(normalizedUnique)
		questionHash := HashNormalized(normalizedQuestion)

		duplicate, err := m.store.IsQuestionDuplicate(uniqueHash, questionHash)
		if err != nil {
			return nil, err
		}
		if duplicate {
			m.logger.Info("Rejected duplicate code question (attempt %d/%d, language=%s difficulty=%s)", attempt, m.generationRetries, language, difficulty)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
			rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
			lastErr = ErrGenerationFailed
			continue
		}

		nearDuplicate, nearReason, err := m.isNearDuplicateCodeQuestion(language, generated.Question, generated.Answer)
		if err != nil {
			return nil, err
		}
		if nearDuplicate {
			m.logger.Info("Rejected near-duplicate code question (attempt %d/%d, language=%s difficulty=%s, reason=%s)", attempt, m.generationRetries, language, difficulty, nearReason)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
			rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
			rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
			lastErr = ErrGenerationFailed
			continue
		}

		stored := &StoredQuestion{
			Mode:           ModeCode,
			Topic:          language,
			Language:       language,
			Question:       generated.Question,
			Answer:         generated.Answer,
			Aliases:        generated.Aliases,
			Hint:           generated.Hint,
			ValidatorType:  generated.ValidatorType,
			UniquenessKey:  normalizedUnique,
			UniquenessHash: uniqueHash,
			QuestionHash:   questionHash,
			CreatedAt:      time.Now(),
		}

		id, err := m.store.InsertQuestion(stored)
		if err != nil {
			lastErr = err
			if isUniqueConstraintErr(err) {
				m.logger.Info("Rejected duplicate code question from unique constraint (attempt %d/%d, language=%s difficulty=%s)", attempt, m.generationRetries, language, difficulty)
				rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedUnique)
				rejectedKeys = appendUniqueRejectedKey(rejectedKeys, normalizedQuestion)
				rejectedQuestions = appendUniqueRejectedQuestion(rejectedQuestions, generated.Question)
				continue
			}
			return nil, err
		}

		stored.ID = id
		return stored, nil
	}

	if lastErr != nil {
		m.logger.Warning("Code generation exhausted retries (language=%s difficulty=%s): %v", language, difficulty, lastErr)
	}
	return nil, ErrGenerationFailed
}

func (m *Manager) handleTimeout(channel string, roundID int64) {
	m.mu.Lock()
	round := m.activeRounds[channel]
	if round == nil || round.closed || round.RoundID != roundID {
		m.mu.Unlock()
		return
	}

	round.closed = true
	delete(m.activeRounds, channel)
	sendMessage := m.sendMessage
	guesses := append([]GuessLog(nil), round.Guesses...)
	m.mu.Unlock()

	if m.shouldRunTimeoutJudge(round, guesses) {
		if sendMessage != nil {
			judgeStart := "Time's up! Checking close answers..."
			if round.Mode == ModeCode {
				judgeStart = "Time's up! Checking close code answers..."
			}
			if err := sendMessage(channel, judgeStart); err != nil {
				m.logger.Warning("Failed to send trivia judge-start message to %s: %v", channel, err)
			}
		}

		judged, err := m.judgeTimeoutWinner(round, guesses)
		if err != nil {
			m.logger.Warning("Trivia timeout judge failed (channel=%s, round_id=%d): %v", channel, roundID, err)
		} else if judged != nil {
			points := calculatePoints(round.Settings, judged.Timestamp.Sub(round.StartedAt), round.HintUsed)
			updatedScore, finalizeErr := m.store.FinalizeRoundWin(
				round.RoundID,
				channel,
				judged.Nick,
				judged.Message,
				points,
				round.HintUsed,
				time.Now(),
			)
			if finalizeErr != nil {
				m.logger.Warning("Failed to persist judged trivia winner (channel=%s, round_id=%d): %v", channel, roundID, finalizeErr)
			} else {
				m.logger.Info(
					"Timeout judged winner: mode=%s channel=%s round_id=%d winner=%s guess_id=%d confidence=%.2f points=%d",
					round.Mode,
					channel,
					roundID,
					judged.Nick,
					judged.ID,
					judged.Confidence,
					points,
				)

				if sendMessage != nil {
					msg := ""
					if round.Mode == ModeCode {
						msg = fmt.Sprintf(
							"Judge accepted %s's close code answer (%q). Official code: %s (+%d points, total: %d).",
							judged.Nick,
							judged.Message,
							round.Answer,
							points,
							updatedScore,
						)
					} else {
						msg = fmt.Sprintf(
							"Judge accepted %s's close answer (%q). Official answer: %s (+%d points, total: %d).",
							judged.Nick,
							judged.Message,
							round.Answer,
							points,
							updatedScore,
						)
					}
					if err := sendMessage(channel, msg); err != nil {
						m.logger.Warning("Failed to send trivia judged-winner message to %s: %v", channel, err)
					}
				}
				return
			}
		}
	}

	if err := m.store.FinalizeRoundNoWinner(round.RoundID, round.HintUsed, "timeout", time.Now()); err != nil {
		m.logger.Warning("Failed to persist trivia timeout (channel=%s, round_id=%d): %v", channel, roundID, err)
	}

	m.logger.Info("Round timeout: mode=%s channel=%s round_id=%d", round.Mode, channel, roundID)

	if sendMessage == nil {
		return
	}

	announcement := fmt.Sprintf("Time's up! The correct answer was: %s", round.Answer)
	if round.Mode == ModeCode {
		announcement = fmt.Sprintf("Time's up! The correct code answer was: %s", round.Answer)
	}
	if err := sendMessage(channel, announcement); err != nil {
		m.logger.Warning("Failed to send trivia timeout message to %s: %v", channel, err)
	}
}

type judgedWinner struct {
	ID         int
	Nick       string
	Message    string
	Timestamp  time.Time
	Confidence float64
}

func (m *Manager) shouldRunTimeoutJudge(round *activeRound, guesses []GuessLog) bool {
	if m.generator == nil || !m.generator.config.Enabled {
		return false
	}
	if len(guesses) == 0 {
		return false
	}
	if round.Mode == ModeCode {
		return true
	}
	return countWords(round.Answer) > longAnswerJudgeWordThreshold
}

func (m *Manager) judgeTimeoutWinner(round *activeRound, guesses []GuessLog) (*judgedWinner, error) {
	candidates := make([]JudgeGuessCandidate, 0, len(guesses))
	for _, guess := range guesses {
		if guess.ID <= 0 {
			continue
		}

		text := strings.TrimSpace(guess.Message)
		if text == "" {
			continue
		}
		if len(text) > 180 {
			text = text[:180] + "..."
		}

		elapsed := guess.Timestamp.Sub(round.StartedAt)
		if elapsed < 0 {
			elapsed = 0
		}

		candidates = append(candidates, JudgeGuessCandidate{
			ID:        guess.ID,
			Nick:      guess.Nick,
			Guess:     text,
			ElapsedMS: elapsed.Milliseconds(),
		})
		if len(candidates) >= maxJudgeCandidates {
			break
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	req := JudgeRequest{
		Mode:       round.Mode,
		Topic:      round.Topic,
		Language:   round.Language,
		Question:   round.Question,
		Answer:     round.Answer,
		Aliases:    append([]string(nil), round.Aliases...),
		Candidates: candidates,
	}

	timeout := 20 * time.Second
	if m.generator.config.RequestTimeout > 0 {
		timeout = m.generator.config.RequestTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	decision, err := m.generator.JudgeClosestGuess(ctx, req)
	if err != nil {
		return nil, err
	}
	if decision == nil || !decision.Approved {
		return nil, nil
	}
	if decision.Confidence < judgeConfidenceThreshold {
		m.logger.Info(
			"Trivia timeout judge rejected due to low confidence (round_id=%d confidence=%.2f threshold=%.2f)",
			round.RoundID,
			decision.Confidence,
			judgeConfidenceThreshold,
		)
		return nil, nil
	}

	for _, guess := range guesses {
		if guess.ID == decision.GuessID {
			return &judgedWinner{
				ID:         guess.ID,
				Nick:       guess.Nick,
				Message:    guess.Message,
				Timestamp:  guess.Timestamp,
				Confidence: decision.Confidence,
			}, nil
		}
	}

	return nil, fmt.Errorf("judge returned unknown guess_id=%d", decision.GuessID)
}

func buildAcceptedAnswerSet(mode, answer string, aliases []string) (map[string]struct{}, string, []string) {
	accepted := make(map[string]struct{}, 1+len(aliases)*2)
	normalizedAliases := make([]string, 0, len(aliases))

	switch NormalizeMode(mode) {
	case ModeCode:
		answerVariants := CodeAnswerVariants(answer)
		for _, variant := range answerVariants {
			accepted[variant] = struct{}{}
		}

		normalizedAnswer := NormalizeCodeAnswer(answer)
		for _, alias := range aliases {
			variants := CodeAnswerVariants(alias)
			for _, variant := range variants {
				accepted[variant] = struct{}{}
			}
			normalizedAlias := NormalizeCodeAnswer(alias)
			if normalizedAlias != "" {
				normalizedAliases = append(normalizedAliases, normalizedAlias)
			}
		}
		return accepted, normalizedAnswer, normalizedAliases
	default:
		normalizedAnswer := NormalizeAnswer(answer)
		if normalizedAnswer != "" {
			accepted[normalizedAnswer] = struct{}{}
		}

		for _, alias := range aliases {
			normalizedAlias := NormalizeAnswer(alias)
			if normalizedAlias == "" {
				continue
			}
			accepted[normalizedAlias] = struct{}{}
			normalizedAliases = append(normalizedAliases, normalizedAlias)
		}
		return accepted, normalizedAnswer, normalizedAliases
	}
}

func isCorrectAnswer(round *activeRound, message string) bool {
	switch NormalizeMode(round.Mode) {
	case ModeCode:
		for _, variant := range CodeAnswerVariants(message) {
			if _, ok := round.AcceptedAnswers[variant]; ok {
				return true
			}
		}
		return false
	default:
		normalized := NormalizeAnswer(message)
		if normalized == "" {
			return false
		}
		_, ok := round.AcceptedAnswers[normalized]
		return ok
	}
}

func calculatePoints(settings ChannelSettings, elapsed time.Duration, hintUsed bool) int {
	answerWindow := time.Duration(settings.AnswerTimeSeconds) * time.Second
	if answerWindow <= 0 {
		answerWindow = 30 * time.Second
	}

	base := settings.BasePoints
	minimum := settings.MinimumPoints
	if base <= 0 {
		base = 100
	}
	if minimum < 0 {
		minimum = 0
	}
	if minimum > base {
		minimum = base
	}

	progress := float64(elapsed) / float64(answerWindow)
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	decayRange := float64(base - minimum)
	points := base - int(math.Round(progress*decayRange))
	if hintUsed {
		points -= settings.HintPenalty
	}
	if points < minimum {
		points = minimum
	}
	return points
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

func appendUniqueRejectedKey(keys []string, key string) []string {
	if key == "" {
		return keys
	}
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func appendUniqueRejectedQuestion(questions []string, question string) []string {
	normalized := NormalizeDedupKey(question)
	if normalized == "" {
		return questions
	}
	for _, existing := range questions {
		if NormalizeDedupKey(existing) == normalized {
			return questions
		}
	}
	return append(questions, question)
}

func (m *Manager) isNearDuplicateQuestion(topic, question, answer string) (bool, string, error) {
	recent, err := m.store.GetRecentQuestionsByTopic(topic, 250)
	if err != nil {
		return false, "", err
	}
	duplicate, reason := isNearDuplicateQuestionInSet(recent, question, answer, NormalizeAnswer)
	return duplicate, reason, nil
}

func (m *Manager) isNearDuplicateCodeQuestion(language, question, answer string) (bool, string, error) {
	recent, err := m.store.GetRecentCodeQuestionsByLanguage(language, 250)
	if err != nil {
		return false, "", err
	}
	duplicate, reason := isNearDuplicateQuestionInSet(recent, question, answer, NormalizeCodeAnswer)
	return duplicate, reason, nil
}

func isNearDuplicateQuestionInSet(recent []historicalQuestion, question, answer string, answerNormalizer func(string) string) (bool, string) {
	if len(recent) == 0 {
		return false, ""
	}

	candidateQuestionTokens := tokenizeForSimilarity(question)
	if len(candidateQuestionTokens) == 0 {
		return false, ""
	}
	candidateAnswer := answerNormalizer(answer)

	for _, existing := range recent {
		existingQuestionTokens := tokenizeForSimilarity(existing.Question)
		if len(existingQuestionTokens) == 0 {
			continue
		}

		questionSimilarity := jaccardSimilarity(candidateQuestionTokens, existingQuestionTokens)
		charSimilarity := trigramJaccardSimilarity(question, existing.Question)
		tokenOverlap := tokenIntersectionCount(candidateQuestionTokens, existingQuestionTokens)
		existingAnswer := answerNormalizer(existing.Answer)
		answerSame := candidateAnswer != "" && existingAnswer != "" && candidateAnswer == existingAnswer

		// Strict near-duplicate based purely on lexical overlap.
		if questionSimilarity >= 0.90 {
			return true, fmt.Sprintf("question_similarity=%.2f", questionSimilarity)
		}

		// Char-level similarity catches minor paraphrases and singular/plural changes.
		if charSimilarity >= 0.78 {
			return true, fmt.Sprintf("char_similarity=%.2f", charSimilarity)
		}

		// Slightly lower threshold when the answer is also the same.
		if answerSame && questionSimilarity >= 0.62 {
			return true, fmt.Sprintf("answer_match=true question_similarity=%.2f", questionSimilarity)
		}

		// Same answer + meaningful token overlap is very likely the same question.
		if answerSame && tokenOverlap >= 2 && questionSimilarity >= 0.45 {
			return true, fmt.Sprintf("answer_match=true token_overlap=%d question_similarity=%.2f", tokenOverlap, questionSimilarity)
		}
	}

	return false, ""
}

func tokenizeForSimilarity(input string) map[string]struct{} {
	normalized := NormalizeDedupKey(input)
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		word = normalizeSimilarityToken(word)
		if word == "" {
			continue
		}
		if _, ignored := ignoredSimilarityTokens[word]; ignored {
			continue
		}
		set[word] = struct{}{}
	}
	return set
}

func normalizeSimilarityToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	switch {
	case strings.HasSuffix(token, "ies") && len(token) > 4:
		token = token[:len(token)-3] + "y"
	case strings.HasSuffix(token, "es") && len(token) > 4:
		token = token[:len(token)-2]
	case strings.HasSuffix(token, "s") && len(token) > 4:
		token = token[:len(token)-1]
	}

	if len(token) <= 1 {
		return ""
	}

	return token
}

func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	union := len(a)

	for key := range b {
		if _, ok := a[key]; ok {
			intersection++
		} else {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func tokenIntersectionCount(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	count := 0
	for key := range a {
		if _, ok := b[key]; ok {
			count++
		}
	}
	return count
}

func trigramJaccardSimilarity(a, b string) float64 {
	na := NormalizeAnswer(a)
	nb := NormalizeAnswer(b)
	if na == "" || nb == "" {
		return 0
	}

	na = strings.ReplaceAll(na, " ", "")
	nb = strings.ReplaceAll(nb, " ", "")

	trigramsA := makeTrigramSet(na)
	trigramsB := makeTrigramSet(nb)
	return jaccardSimilarity(trigramsA, trigramsB)
}

func makeTrigramSet(input string) map[string]struct{} {
	if len(input) < 3 {
		set := make(map[string]struct{}, 1)
		set[input] = struct{}{}
		return set
	}

	set := make(map[string]struct{}, len(input)-2)
	for i := 0; i <= len(input)-3; i++ {
		set[input[i:i+3]] = struct{}{}
	}
	return set
}
