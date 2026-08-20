-- Schema for domain.Highlight persistence (internal/adapters/postgres.HighlightRepository).
CREATE TABLE IF NOT EXISTS highlights (
    id          TEXT PRIMARY KEY,
    book_id     TEXT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL,
    box_x       DOUBLE PRECISION NOT NULL,
    box_y       DOUBLE PRECISION NOT NULL,
    box_width   DOUBLE PRECISION NOT NULL,
    box_height  DOUBLE PRECISION NOT NULL,
    color       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_highlights_book_id ON highlights (book_id);
