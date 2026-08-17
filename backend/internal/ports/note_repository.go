package ports

import (
	"context"

	"pdf-reader/backend/internal/domain"
)

// NoteRepository persists and retrieves Note records.
type NoteRepository interface {
	// Create stores a new Note.
	Create(ctx context.Context, note *domain.Note) error

	// FindByID returns the Note with the given ID, or an error if it does not exist.
	FindByID(ctx context.Context, id string) (*domain.Note, error)

	// ListByBookID returns all Notes belonging to the given Book.
	ListByBookID(ctx context.Context, bookID string) ([]*domain.Note, error)

	// Update persists changes made to an existing Note.
	Update(ctx context.Context, note *domain.Note) error

	// Delete removes the Note with the given ID.
	Delete(ctx context.Context, id string) error
}
