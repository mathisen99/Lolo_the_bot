ALTER TABLE trivia_settings
    ADD COLUMN code_answer_time_seconds INTEGER NOT NULL DEFAULT 30;

ALTER TABLE trivia_settings
    ADD COLUMN code_difficulty TEXT NOT NULL DEFAULT 'medium';

UPDATE trivia_settings
SET code_answer_time_seconds = answer_time_seconds
WHERE code_answer_time_seconds <= 0;

UPDATE trivia_settings
SET code_difficulty = difficulty
WHERE TRIM(COALESCE(code_difficulty, '')) = '';
