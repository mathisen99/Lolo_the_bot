ALTER TABLE trivia_settings
    ADD COLUMN difficulty TEXT NOT NULL DEFAULT 'medium';

UPDATE trivia_settings
SET difficulty = 'medium'
WHERE TRIM(COALESCE(difficulty, '')) = '';
