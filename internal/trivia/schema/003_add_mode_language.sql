ALTER TABLE trivia_questions
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'trivia';

ALTER TABLE trivia_questions
    ADD COLUMN language TEXT NOT NULL DEFAULT '';

ALTER TABLE trivia_questions
    ADD COLUMN validator_type TEXT NOT NULL DEFAULT 'normalized_exact';

UPDATE trivia_questions
SET mode = 'trivia'
WHERE TRIM(COALESCE(mode, '')) = '';

UPDATE trivia_questions
SET language = ''
WHERE language IS NULL;

UPDATE trivia_questions
SET validator_type = 'normalized_exact'
WHERE TRIM(COALESCE(validator_type, '')) = '';

CREATE INDEX IF NOT EXISTS idx_trivia_questions_mode_topic_created
    ON trivia_questions (mode, topic, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_trivia_questions_mode_language_created
    ON trivia_questions (mode, language, created_at DESC);

ALTER TABLE trivia_rounds
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'trivia';

ALTER TABLE trivia_rounds
    ADD COLUMN language TEXT NOT NULL DEFAULT '';

UPDATE trivia_rounds
SET mode = 'trivia'
WHERE TRIM(COALESCE(mode, '')) = '';

UPDATE trivia_rounds
SET language = ''
WHERE language IS NULL;
