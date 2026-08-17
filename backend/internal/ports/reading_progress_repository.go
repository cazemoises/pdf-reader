package ports

import (
	"context"

	"pdf-reader/backend/internal/domain"
)

// ReadingProgressRepository persists and retrieves a Book's ReadingProgress.
type ReadingProgressRepository interface {
	// GetByBookID returns the ReadingProgress for the given Book, or an error if none exists.
	GetByBookID(ctx context.Context, bookID string) (*domain.ReadingProgress, error)

	// Save creates or overwrites the ReadingProgress for its Book.
	Save(ctx context.Context, progress *domain.ReadingProgress) error
}
