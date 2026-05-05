ALTER TABLE trivia_rounds
    ADD COLUMN network TEXT NOT NULL DEFAULT 'libera';

CREATE INDEX IF NOT EXISTS idx_trivia_rounds_network_channel_started
    ON trivia_rounds (network, channel, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_trivia_rounds_network_channel_variant_started
    ON trivia_rounds (network, channel, variant, started_at DESC);

CREATE TABLE trivia_scores_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network TEXT NOT NULL DEFAULT 'libera',
    channel TEXT NOT NULL,
    nick TEXT COLLATE NOCASE NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL,
    UNIQUE(network, channel, nick)
);

INSERT INTO trivia_scores_new (id, network, channel, nick, score, updated_at)
SELECT id, 'libera', channel, nick, score, updated_at
FROM trivia_scores;

DROP TABLE trivia_scores;
ALTER TABLE trivia_scores_new RENAME TO trivia_scores;

CREATE INDEX IF NOT EXISTS idx_trivia_scores_network_channel_score
    ON trivia_scores (network, channel, score DESC, nick ASC);

CREATE TABLE trivia_settings_new (
    network TEXT NOT NULL DEFAULT 'libera',
    channel TEXT NOT NULL,
    answer_time_seconds INTEGER NOT NULL,
    code_answer_time_seconds INTEGER NOT NULL DEFAULT 30,
    hints_enabled INTEGER NOT NULL,
    trivia_hints_enabled INTEGER NOT NULL DEFAULT 1,
    code_hints_enabled INTEGER NOT NULL DEFAULT 1,
    base_points INTEGER NOT NULL,
    minimum_points INTEGER NOT NULL,
    hint_penalty INTEGER NOT NULL,
    enabled INTEGER NOT NULL,
    difficulty TEXT NOT NULL DEFAULT 'medium',
    code_difficulty TEXT NOT NULL DEFAULT 'medium',
    updated_at DATETIME NOT NULL,
    PRIMARY KEY(network, channel)
);

INSERT INTO trivia_settings_new (
    network, channel, answer_time_seconds, code_answer_time_seconds, hints_enabled, trivia_hints_enabled,
    code_hints_enabled, base_points, minimum_points, hint_penalty, enabled, difficulty, code_difficulty, updated_at
)
SELECT
    'libera',
    channel,
    answer_time_seconds,
    code_answer_time_seconds,
    hints_enabled,
    trivia_hints_enabled,
    code_hints_enabled,
    base_points,
    minimum_points,
    hint_penalty,
    enabled,
    difficulty,
    code_difficulty,
    updated_at
FROM trivia_settings;

DROP TABLE trivia_settings;
ALTER TABLE trivia_settings_new RENAME TO trivia_settings;
