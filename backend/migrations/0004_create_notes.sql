-- Schema for domain.Note persistence (internal/adapters/postgres.NoteRepository).
CREATE TABLE IF NOT EXISTS notes (
    id           TEXT PRIMARY KEY,
    book_id      TEXT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    highlight_id TEXT NULL REFERENCES highlights (id) ON DELETE CASCADE,
    content      TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_book_id ON notes (book_id);
