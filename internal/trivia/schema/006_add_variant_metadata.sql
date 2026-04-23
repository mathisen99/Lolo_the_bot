ALTER TABLE trivia_questions
    ADD COLUMN variant TEXT NOT NULL DEFAULT 'classic';

ALTER TABLE trivia_questions
    ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';

UPDATE trivia_questions
SET variant = 'classic'
WHERE COALESCE(TRIM(variant), '') = '';

UPDATE trivia_questions
SET metadata_json = '{}'
WHERE COALESCE(TRIM(metadata_json), '') = '';

ALTER TABLE trivia_rounds
    ADD COLUMN variant TEXT NOT NULL DEFAULT 'classic';

ALTER TABLE trivia_rounds
    ADD COLUMN modifiers_json TEXT NOT NULL DEFAULT '[]';

UPDATE trivia_rounds
SET variant = 'classic'
WHERE COALESCE(TRIM(variant), '') = '';

UPDATE trivia_rounds
SET modifiers_json = '[]'
WHERE COALESCE(TRIM(modifiers_json), '') = '';

CREATE INDEX IF NOT EXISTS idx_trivia_rounds_channel_variant_started
    ON trivia_rounds (channel, variant, started_at DESC);
