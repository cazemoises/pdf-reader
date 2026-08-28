ALTER TABLE reading_progress
    ADD COLUMN IF NOT EXISTS page_number INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS character_offset INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'auto';

UPDATE reading_progress
SET page_number = last_page
WHERE page_number = 1 AND last_page <> 1;

ALTER TABLE reading_progress
    ALTER COLUMN page_number DROP DEFAULT,
    ALTER COLUMN character_offset DROP DEFAULT,
    ALTER COLUMN source DROP DEFAULT;
