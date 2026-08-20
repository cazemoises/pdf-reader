// Package postgres implements ports.HighlightRepository backed by a Postgres
// database via the standard library database/sql package.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// ErrHighlightNotFound is returned (wrapped) when a Highlight lookup,
// update, or delete targets an id that does not exist in the highlights
// table.
var ErrHighlightNotFound = errors.New("postgres: highlight not found")

// HighlightRepository implements ports.HighlightRepository against a
// highlights table in Postgres.
type HighlightRepository struct {
	db *sql.DB
}

// NewHighlightRepository creates a HighlightRepository using db as its
// connection pool. db is not owned by the repository; callers remain
// responsible for closing it.
func NewHighlightRepository(db *sql.DB) *HighlightRepository {
	return &HighlightRepository{db: db}
}

// Create stores a new Highlight.
func (r *HighlightRepository) Create(ctx context.Context, highlight *domain.Highlight) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO highlights (id, book_id, page_number, box_x, box_y, box_width, box_height, color, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		highlight.ID, highlight.BookID, highlight.PageNumber,
		highlight.Box.X, highlight.Box.Y, highlight.Box.Width, highlight.Box.Height,
		highlight.Color, highlight.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: creating highlight: %w", err)
	}
	return nil
}

// FindByID returns the Highlight with the given ID, or an error wrapping
// ErrHighlightNotFound if it does not exist.
func (r *HighlightRepository) FindByID(ctx context.Context, id string) (*domain.Highlight, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, book_id, page_number, box_x, box_y, box_width, box_height, color, created_at
		 FROM highlights WHERE id = $1`, id)

	highlight, err := scanHighlight(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: highlight %q: %w", id, ErrHighlightNotFound)
		}
		return nil, fmt.Errorf("postgres: finding highlight: %w", err)
	}
	return highlight, nil
}

// ListByBookID returns all Highlights belonging to the given Book.
func (r *HighlightRepository) ListByBookID(ctx context.Context, bookID string) ([]*domain.Highlight, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, book_id, page_number, box_x, box_y, box_width, box_height, color, created_at
		 FROM highlights WHERE book_id = $1`, bookID)
	if err != nil {
		return nil, fmt.Errorf("postgres: listing highlights: %w", err)
	}
	defer rows.Close()

	highlights := make([]*domain.Highlight, 0)
	for rows.Next() {
		highlight, err := scanHighlight(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scanning highlight: %w", err)
		}
		highlights = append(highlights, highlight)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: listing highlights: %w", err)
	}
	return highlights, nil
}

// Update persists changes made to an existing Highlight. It returns an
// error wrapping ErrHighlightNotFound if no Highlight with the given ID
// exists.
func (r *HighlightRepository) Update(ctx context.Context, highlight *domain.Highlight) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE highlights
		 SET book_id = $2, page_number = $3, box_x = $4, box_y = $5, box_width = $6, box_height = $7,
		     color = $8, created_at = $9
		 WHERE id = $1`,
		highlight.ID, highlight.BookID, highlight.PageNumber,
		highlight.Box.X, highlight.Box.Y, highlight.Box.Width, highlight.Box.Height,
		highlight.Color, highlight.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: updating highlight: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: updating highlight %q: %w", highlight.ID, ErrHighlightNotFound)
	}
	return nil
}

// Delete removes the Highlight with the given ID. It returns an error
// wrapping ErrHighlightNotFound if no Highlight with the given ID exists.
func (r *HighlightRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM highlights WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: deleting highlight: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: deleting highlight %q: %w", id, ErrHighlightNotFound)
	}
	return nil
}

func scanHighlight(s rowScanner) (*domain.Highlight, error) {
	var highlight domain.Highlight
	if err := s.Scan(
		&highlight.ID, &highlight.BookID, &highlight.PageNumber,
		&highlight.Box.X, &highlight.Box.Y, &highlight.Box.Width, &highlight.Box.Height,
		&highlight.Color, &highlight.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &highlight, nil
}
