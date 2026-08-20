package ports

import (
	"context"

	"pdf-reader/backend/internal/domain"
)

// PageRepository persists and retrieves Page records.
type PageRepository interface {
	// Create stores a new Page.
	Create(ctx context.Context, page *domain.Page) error

	// ListByBookID returns all Pages belonging to the given Book, ordered by
	// page number.
	ListByBookID(ctx context.Context, bookID string) ([]*domain.Page, error)
}
