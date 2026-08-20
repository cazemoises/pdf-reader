// Package postgres implements ports.PageRepository backed by a Postgres
// database via the standard library database/sql package.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// PageRepository implements ports.PageRepository against a pages table in
// Postgres.
type PageRepository struct {
	db *sql.DB
}

// NewPageRepository creates a PageRepository using db as its connection
// pool. db is not owned by the repository; callers remain responsible for
// closing it.
func NewPageRepository(db *sql.DB) *PageRepository {
	return &PageRepository{db: db}
}

// Create stores a new Page.
func (r *PageRepository) Create(ctx context.Context, page *domain.Page) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO pages (book_id, number, text, width, height)
		 VALUES ($1, $2, $3, $4, $5)`,
		page.BookID, page.Number, page.Text, page.Width, page.Height,
	)
	if err != nil {
		return fmt.Errorf("postgres: creating page: %w", err)
	}
	return nil
}

// ListByBookID returns all Pages belonging to the given Book, ordered by
// page number.
func (r *PageRepository) ListByBookID(ctx context.Context, bookID string) ([]*domain.Page, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT book_id, number, text, width, height
		 FROM pages WHERE book_id = $1 ORDER BY number`, bookID)
	if err != nil {
		return nil, fmt.Errorf("postgres: listing pages: %w", err)
	}
	defer rows.Close()

	pages := make([]*domain.Page, 0)
	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.BookID, &page.Number, &page.Text, &page.Width, &page.Height); err != nil {
			return nil, fmt.Errorf("postgres: scanning page: %w", err)
		}
		pages = append(pages, &page)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: listing pages: %w", err)
	}
	return pages, nil
}
