-- Schema for domain.Page persistence (internal/adapters/postgres.PageRepository).
CREATE TABLE IF NOT EXISTS pages (
    book_id TEXT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    number  INTEGER NOT NULL,
    text    TEXT NOT NULL,
    width   DOUBLE PRECISION NOT NULL,
    height  DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (book_id, number)
);
