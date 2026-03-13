package trivia

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides persistent trivia storage in a dedicated SQLite database.
type Store struct {
	conn     *sql.DB
	path     string
	defaults StoreDefaults
}

// NewStore creates a trivia database connection and applies migrations.
func NewStore(path string, defaults StoreDefaults) (*Store, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create trivia data directory: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open trivia database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping trivia database: %w", err)
	}

	store := &Store{
		conn:     conn,
		path:     path,
		defaults: defaults,
	}

	if err := store.configureSQLite(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := store.runMigrations(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the trivia database connection.
func (s *Store) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *Store) configureSQLite() error {
	if s.path != ":memory:" {
		var journalMode string
		if err := s.conn.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
			return fmt.Errorf("failed to enable trivia WAL mode: %w", err)
		}
		if journalMode != "wal" {
			return fmt.Errorf("failed to enable trivia WAL mode: got %s", journalMode)
		}
	}

	if _, err := s.conn.Exec("PRAGMA wal_autocheckpoint=5000"); err != nil {
		return fmt.Errorf("failed to configure trivia wal_autocheckpoint: %w", err)
	}
	if _, err := s.conn.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return fmt.Errorf("failed to configure trivia synchronous mode: %w", err)
	}
	if _, err := s.conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to configure trivia busy timeout: %w", err)
	}
	return nil
}

// IsQuestionDuplicate checks dedup hashes in persisted questions.
func (s *Store) IsQuestionDuplicate(uniqueHash, questionHash string) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM trivia_questions
		WHERE uniqueness_hash IN (?, ?)
		   OR question_hash IN (?, ?)
	`
	var count int
	err := s.conn.QueryRow(query, uniqueHash, questionHash, uniqueHash, questionHash).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check question duplicate: %w", err)
	}
	return count > 0, nil
}

// InsertQuestion stores a generated trivia question.
func (s *Store) InsertQuestion(q *StoredQuestion) (int64, error) {
	aliasesJSON, err := json.Marshal(q.Aliases)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal aliases: %w", err)
	}

	result, err := s.conn.Exec(`
		INSERT INTO trivia_questions (
			topic, question, answer, aliases_json, hint, uniqueness_key, uniqueness_hash, question_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		q.Topic,
		q.Question,
		q.Answer,
		string(aliasesJSON),
		q.Hint,
		q.UniquenessKey,
		q.UniquenessHash,
		q.QuestionHash,
		q.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert trivia question: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get trivia question id: %w", err)
	}
	return id, nil
}

// StartRound inserts a new active round.
func (s *Store) StartRound(channel, topic string, questionID int64, startedAt time.Time) (int64, error) {
	result, err := s.conn.Exec(`
		INSERT INTO trivia_rounds (
			channel, topic, question_id, started_at, status
		) VALUES (?, ?, ?, ?, ?)
	`, channel, topic, questionID, startedAt, "active")
	if err != nil {
		return 0, fmt.Errorf("failed to insert trivia round: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get trivia round id: %w", err)
	}
	return id, nil
}

// FinalizeRoundWin persists winner details and updates score atomically.
func (s *Store) FinalizeRoundWin(roundID int64, channel, winnerNick, winningAnswer string, points int, hintUsed bool, endedAt time.Time) (int, error) {
	tx, err := s.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin trivia win transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := s.addScoreTx(tx, channel, winnerNick, points); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(`
		UPDATE trivia_rounds
		SET ended_at = ?, winner_nick = ?, winning_answer = ?, points_awarded = ?, hint_used = ?, status = ?
		WHERE id = ?
	`,
		endedAt,
		winnerNick,
		winningAnswer,
		points,
		boolToInt(hintUsed),
		"completed",
		roundID,
	); err != nil {
		return 0, fmt.Errorf("failed to complete trivia round: %w", err)
	}

	var updatedScore int
	if err := tx.QueryRow(`
		SELECT score FROM trivia_scores
		WHERE channel = ? AND nick = ?
	`, channel, winnerNick).Scan(&updatedScore); err != nil {
		return 0, fmt.Errorf("failed to fetch updated trivia score: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit trivia win transaction: %w", err)
	}

	return updatedScore, nil
}

// FinalizeRoundNoWinner persists a round that ended without a winner.
func (s *Store) FinalizeRoundNoWinner(roundID int64, hintUsed bool, status string, endedAt time.Time) error {
	if _, err := s.conn.Exec(`
		UPDATE trivia_rounds
		SET ended_at = ?, points_awarded = 0, hint_used = ?, status = ?
		WHERE id = ?
	`, endedAt, boolToInt(hintUsed), status, roundID); err != nil {
		return fmt.Errorf("failed to finalize trivia round with no winner: %w", err)
	}
	return nil
}

// GetTopScores returns top scores for a channel.
func (s *Store) GetTopScores(channel string, limit int) ([]ScoreEntry, error) {
	rows, err := s.conn.Query(`
		SELECT nick, score
		FROM trivia_scores
		WHERE channel = ?
		ORDER BY score DESC, nick COLLATE NOCASE ASC
		LIMIT ?
	`, channel, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top scores: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	entries := make([]ScoreEntry, 0, limit)
	for rows.Next() {
		var entry ScoreEntry
		if err := rows.Scan(&entry.Nick, &entry.Score); err != nil {
			return nil, fmt.Errorf("failed to scan top score row: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed while iterating top scores: %w", err)
	}

	return entries, nil
}

// GetScore returns a nick's score in a channel.
func (s *Store) GetScore(channel, nick string) (int, bool, error) {
	var score int
	err := s.conn.QueryRow(`
		SELECT score
		FROM trivia_scores
		WHERE channel = ? AND nick = ?
	`, channel, nick).Scan(&score)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to get trivia score: %w", err)
	}
	return score, true, nil
}

// SetScore sets a score to an exact value for a nick in a channel.
func (s *Store) SetScore(channel, nick string, score int) error {
	if score < 0 {
		score = 0
	}
	_, err := s.conn.Exec(`
		INSERT INTO trivia_scores (channel, nick, score, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(channel, nick) DO UPDATE SET
			nick = excluded.nick,
			score = excluded.score,
			updated_at = excluded.updated_at
	`, channel, nick, score, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set trivia score: %w", err)
	}
	return nil
}

// AddScore adds/subtracts points and returns the resulting score.
func (s *Store) AddScore(channel, nick string, delta int) (int, error) {
	tx, err := s.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin trivia score transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var current int
	err = tx.QueryRow(`
		SELECT score
		FROM trivia_scores
		WHERE channel = ? AND nick = ?
	`, channel, nick).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to read current trivia score: %w", err)
	}

	if err == sql.ErrNoRows {
		current = 0
	}

	next := current + delta
	if next < 0 {
		next = 0
	}

	if _, err := tx.Exec(`
		INSERT INTO trivia_scores (channel, nick, score, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(channel, nick) DO UPDATE SET
			nick = excluded.nick,
			score = excluded.score,
			updated_at = excluded.updated_at
	`, channel, nick, next, time.Now()); err != nil {
		return 0, fmt.Errorf("failed to update trivia score: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit trivia score transaction: %w", err)
	}

	return next, nil
}

// ResetScore removes a score row for a nick in a channel.
func (s *Store) ResetScore(channel, nick string) error {
	if _, err := s.conn.Exec(`
		DELETE FROM trivia_scores
		WHERE channel = ? AND nick = ?
	`, channel, nick); err != nil {
		return fmt.Errorf("failed to reset trivia score: %w", err)
	}
	return nil
}

// GetSettings loads channel settings, returning defaults if none are persisted.
func (s *Store) GetSettings(channel string) (ChannelSettings, error) {
	settings := s.defaults.Settings

	var hintsEnabled int
	var enabled int

	err := s.conn.QueryRow(`
		SELECT answer_time_seconds, hints_enabled, base_points, minimum_points, hint_penalty, enabled
		FROM trivia_settings
		WHERE channel = ?
	`, channel).Scan(
		&settings.AnswerTimeSeconds,
		&hintsEnabled,
		&settings.BasePoints,
		&settings.MinimumPoints,
		&settings.HintPenalty,
		&enabled,
	)

	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return ChannelSettings{}, fmt.Errorf("failed to get trivia settings: %w", err)
	}

	settings.HintsEnabled = hintsEnabled == 1
	settings.Enabled = enabled == 1
	return settings, nil
}

// SaveSettings upserts settings for a channel.
func (s *Store) SaveSettings(channel string, settings ChannelSettings) error {
	_, err := s.conn.Exec(`
		INSERT INTO trivia_settings (
			channel, answer_time_seconds, hints_enabled, base_points, minimum_points, hint_penalty, enabled, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel) DO UPDATE SET
			answer_time_seconds = excluded.answer_time_seconds,
			hints_enabled = excluded.hints_enabled,
			base_points = excluded.base_points,
			minimum_points = excluded.minimum_points,
			hint_penalty = excluded.hint_penalty,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		channel,
		settings.AnswerTimeSeconds,
		boolToInt(settings.HintsEnabled),
		settings.BasePoints,
		settings.MinimumPoints,
		settings.HintPenalty,
		boolToInt(settings.Enabled),
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save trivia settings: %w", err)
	}
	return nil
}

func (s *Store) addScoreTx(tx *sql.Tx, channel, nick string, delta int) error {
	var current int
	err := tx.QueryRow(`
		SELECT score
		FROM trivia_scores
		WHERE channel = ? AND nick = ?
	`, channel, nick).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to read score in transaction: %w", err)
	}
	if err == sql.ErrNoRows {
		current = 0
	}

	next := current + delta
	if next < 0 {
		next = 0
	}

	if _, err := tx.Exec(`
		INSERT INTO trivia_scores (channel, nick, score, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(channel, nick) DO UPDATE SET
			nick = excluded.nick,
			score = excluded.score,
			updated_at = excluded.updated_at
	`, channel, nick, next, time.Now()); err != nil {
		return fmt.Errorf("failed to write score in transaction: %w", err)
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
