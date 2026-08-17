package ports

import (
	"context"

	"pdf-reader/backend/internal/domain"
)

// BookRepository persists and retrieves Book records.
type BookRepository interface {
	// Create stores a new Book.
	Create(ctx context.Context, book *domain.Book) error

	// FindByID returns the Book with the given ID, or an error if it does not exist.
	FindByID(ctx context.Context, id string) (*domain.Book, error)

	// Update persists changes made to an existing Book (e.g. its Status).
	Update(ctx context.Context, book *domain.Book) error

	// List returns all known Books.
	List(ctx context.Context) ([]*domain.Book, error)

	// Delete removes the Book with the given ID.
	Delete(ctx context.Context, id string) error
}
