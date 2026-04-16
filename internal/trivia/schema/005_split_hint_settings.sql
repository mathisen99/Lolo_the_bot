ALTER TABLE trivia_settings
    ADD COLUMN trivia_hints_enabled INTEGER NOT NULL DEFAULT 1;

ALTER TABLE trivia_settings
    ADD COLUMN code_hints_enabled INTEGER NOT NULL DEFAULT 1;

UPDATE trivia_settings
SET trivia_hints_enabled = hints_enabled;

UPDATE trivia_settings
SET code_hints_enabled = hints_enabled;
