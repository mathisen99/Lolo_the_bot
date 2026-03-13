CREATE TABLE IF NOT EXISTS trivia_questions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    topic TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    aliases_json TEXT NOT NULL DEFAULT '[]',
    hint TEXT NOT NULL,
    uniqueness_key TEXT NOT NULL,
    uniqueness_hash TEXT NOT NULL,
    question_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trivia_questions_uniqueness_hash
    ON trivia_questions (uniqueness_hash);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trivia_questions_question_hash
    ON trivia_questions (question_hash);

CREATE TABLE IF NOT EXISTS trivia_rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    topic TEXT NOT NULL,
    question_id INTEGER NOT NULL,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    winner_nick TEXT,
    winning_answer TEXT,
    points_awarded INTEGER NOT NULL DEFAULT 0,
    hint_used INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    FOREIGN KEY (question_id) REFERENCES trivia_questions(id)
);

CREATE INDEX IF NOT EXISTS idx_trivia_rounds_channel_started
    ON trivia_rounds (channel, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_trivia_rounds_status
    ON trivia_rounds (status);

CREATE TABLE IF NOT EXISTS trivia_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    nick TEXT COLLATE NOCASE NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL,
    UNIQUE (channel, nick)
);

CREATE INDEX IF NOT EXISTS idx_trivia_scores_channel_score
    ON trivia_scores (channel, score DESC, nick ASC);

CREATE TABLE IF NOT EXISTS trivia_settings (
    channel TEXT PRIMARY KEY,
    answer_time_seconds INTEGER NOT NULL,
    hints_enabled INTEGER NOT NULL,
    base_points INTEGER NOT NULL,
    minimum_points INTEGER NOT NULL,
    hint_penalty INTEGER NOT NULL,
    enabled INTEGER NOT NULL,
    updated_at DATETIME NOT NULL
);
