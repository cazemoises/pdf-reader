-- Highlights are now anchored to a page's plain text (as served by
-- GET /books/{id}/pages/{number}) instead of a canvas-rendered PDF, so the
-- geometric bounding box columns no longer describe anything meaningful.
-- Replace them with a character offset range within that plain text.
-- (See docs/superpowers/plans/2026-08-21-fluid-reflow-reader.md for why
-- this replaces the box columns instead of reinterpreting them.)
ALTER TABLE highlights DROP COLUMN IF EXISTS box_x;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_y;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_width;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_height;
ALTER TABLE highlights ADD COLUMN IF NOT EXISTS start_offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE highlights ADD COLUMN IF NOT EXISTS end_offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE highlights ALTER COLUMN start_offset DROP DEFAULT;
ALTER TABLE highlights ALTER COLUMN end_offset DROP DEFAULT;
