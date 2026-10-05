-- Additive: existing pages remain unchanged. New imports retain layout, provenance and diagnostics.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS extraction JSONB;
