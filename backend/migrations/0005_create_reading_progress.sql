-- Schema for domain.ReadingProgress persistence (internal/adapters/postgres.ReadingProgressRepository).
CREATE TABLE IF NOT EXISTS reading_progress (
    book_id    TEXT PRIMARY KEY REFERENCES books (id) ON DELETE CASCADE,
    last_page  INTEGER NOT NULL,
    percentage DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
