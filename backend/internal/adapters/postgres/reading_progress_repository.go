package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// ErrReadingProgressNotFound is returned (wrapped) when a ReadingProgress
// lookup targets a book id that has no progress saved in the
// reading_progress table.
var ErrReadingProgressNotFound = errors.New("postgres: reading progress not found")

// ReadingProgressRepository implements ports.ReadingProgressRepository
// against a reading_progress table in Postgres.
type ReadingProgressRepository struct {
	db *sql.DB
}

// NewReadingProgressRepository creates a ReadingProgressRepository using db
// as its connection pool. db is not owned by the repository; callers remain
// responsible for closing it.
func NewReadingProgressRepository(db *sql.DB) *ReadingProgressRepository {
	return &ReadingProgressRepository{db: db}
}

// GetByBookID returns the ReadingProgress for the given Book, or an error
// wrapping ErrReadingProgressNotFound if none exists.
func (r *ReadingProgressRepository) GetByBookID(ctx context.Context, bookID string) (*domain.ReadingProgress, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT book_id, last_page, percentage, updated_at
		 FROM reading_progress WHERE book_id = $1`, bookID)

	progress, err := scanReadingProgress(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: reading progress for book %q: %w", bookID, ErrReadingProgressNotFound)
		}
		return nil, fmt.Errorf("postgres: finding reading progress: %w", err)
	}
	return progress, nil
}

// Save creates or overwrites the ReadingProgress for its Book.
func (r *ReadingProgressRepository) Save(ctx context.Context, progress *domain.ReadingProgress) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO reading_progress (book_id, last_page, percentage, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (book_id) DO UPDATE
		 SET last_page = EXCLUDED.last_page,
		     percentage = EXCLUDED.percentage,
		     updated_at = EXCLUDED.updated_at`,
		progress.BookID, progress.LastPage, progress.Percentage, progress.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: saving reading progress: %w", err)
	}
	return nil
}

func scanReadingProgress(s rowScanner) (*domain.ReadingProgress, error) {
	var progress domain.ReadingProgress
	if err := s.Scan(&progress.BookID, &progress.LastPage, &progress.Percentage, &progress.UpdatedAt); err != nil {
		return nil, err
	}
	return &progress, nil
}
