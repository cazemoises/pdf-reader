package ports

import (
	"context"

	"pdf-reader/backend/internal/domain"
)

// HighlightRepository persists and retrieves Highlight records.
type HighlightRepository interface {
	// Create stores a new Highlight.
	Create(ctx context.Context, highlight *domain.Highlight) error

	// FindByID returns the Highlight with the given ID, or an error if it does not exist.
	FindByID(ctx context.Context, id string) (*domain.Highlight, error)

	// ListByBookID returns all Highlights belonging to the given Book.
	ListByBookID(ctx context.Context, bookID string) ([]*domain.Highlight, error)

	// Update persists changes made to an existing Highlight.
	Update(ctx context.Context, highlight *domain.Highlight) error

	// Delete removes the Highlight with the given ID.
	Delete(ctx context.Context, id string) error
}
