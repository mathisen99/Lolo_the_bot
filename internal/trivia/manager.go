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

type activeRound struct {
	RoundID           int64
	Channel           string
	Topic             string
	QuestionID        int64
	Question          string
	Answer            string
	Hint              string
	StartedAt         time.Time
	AcceptedAnswers   map[string]struct{}
	Settings          ChannelSettings
	HintUsed          bool
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

	question, err := m.generateAndPersistQuestion(ctx, topic)
	if err != nil {
		return "", err
	}

	accepted := make(map[string]struct{}, 1+len(question.Aliases))
	normalizedAnswer := NormalizeAnswer(question.Answer)
	if normalizedAnswer == "" {
		return "", ErrGenerationFailed
	}
	accepted[normalizedAnswer] = struct{}{}

	normalizedAliases := make([]string, 0, len(question.Aliases))
	for _, alias := range question.Aliases {
		normalizedAlias := NormalizeAnswer(alias)
		if normalizedAlias == "" {
			continue
		}
		accepted[normalizedAlias] = struct{}{}
		normalizedAliases = append(normalizedAliases, normalizedAlias)
	}

	startedAt := time.Now()
	roundID, err := m.store.StartRound(channel, topic, question.ID, startedAt)
	if err != nil {
		return "", err
	}

	round := &activeRound{
		RoundID:           roundID,
		Channel:           channel,
		Topic:             topic,
		QuestionID:        question.ID,
		Question:          question.Question,
		Answer:            question.Answer,
		Hint:              question.Hint,
		StartedAt:         startedAt,
		AcceptedAnswers:   accepted,
		Settings:          settings,
		HintUsed:          false,
		closed:            false,
		NormalizedAnswer:  normalizedAnswer,
		NormalizedAliases: normalizedAliases,
	}

	duration := time.Duration(settings.AnswerTimeSeconds) * time.Second
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

	m.logger.Info("Trivia round started: channel=%s topic=%s question_id=%d round_id=%d", channel, topic, question.ID, roundID)

	msg := fmt.Sprintf(
		"Trivia (%s): %s | You have %ds. Answer with normal channel text. Use !hint for a hint.",
		topic,
		question.Question,
		settings.AnswerTimeSeconds,
	)
	return msg, nil
}

// TryAnswer checks a normal channel message against active round answers.
// Returns response text and handled=true only when the message wins the round.
func (m *Manager) TryAnswer(channel, nick, message string) (string, bool, error) {
	normalized := NormalizeAnswer(message)
	if normalized == "" {
		return "", false, nil
	}

	m.mu.Lock()
	round := m.activeRounds[channel]
	if round == nil || round.closed {
		m.mu.Unlock()
		return "", false, nil
	}

	if _, ok := round.AcceptedAnswers[normalized]; !ok {
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

	m.logger.Info("Trivia winner: channel=%s round_id=%d winner=%s points=%d", channel, round.RoundID, nick, points)

	response := fmt.Sprintf(
		"%s got it! Answer: %s (+%d points, total: %d).",
		nick,
		round.Answer,
		points,
		updatedScore,
	)
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

func (m *Manager) generateAndPersistQuestion(ctx context.Context, topic string) (*StoredQuestion, error) {
	if m.generator == nil {
		return nil, ErrGeneratorDisabled
	}

	var lastErr error
	for attempt := 1; attempt <= m.generationRetries; attempt++ {
		generated, err := m.generator.GenerateQuestion(ctx, topic)
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
			lastErr = ErrGenerationFailed
			continue
		}

		stored := &StoredQuestion{
			Topic:          topic,
			Question:       generated.Question,
			Answer:         generated.Answer,
			Aliases:        generated.Aliases,
			Hint:           generated.Hint,
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
	m.mu.Unlock()

	if err := m.store.FinalizeRoundNoWinner(round.RoundID, round.HintUsed, "timeout", time.Now()); err != nil {
		m.logger.Warning("Failed to persist trivia timeout (channel=%s, round_id=%d): %v", channel, roundID, err)
	}

	m.logger.Info("Trivia timeout: channel=%s round_id=%d", channel, roundID)

	if sendMessage == nil {
		return
	}

	announcement := fmt.Sprintf("Time's up! The correct answer was: %s", round.Answer)
	if err := sendMessage(channel, announcement); err != nil {
		m.logger.Warning("Failed to send trivia timeout message to %s: %v", channel, err)
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
