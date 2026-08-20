package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// ErrNoteNotFound is returned (wrapped) when a Note lookup, update, or
// delete targets an id that does not exist in the notes table.
var ErrNoteNotFound = errors.New("postgres: note not found")

// NoteRepository implements ports.NoteRepository against a notes table in
// Postgres.
type NoteRepository struct {
	db *sql.DB
}

// NewNoteRepository creates a NoteRepository using db as its connection
// pool. db is not owned by the repository; callers remain responsible for
// closing it.
func NewNoteRepository(db *sql.DB) *NoteRepository {
	return &NoteRepository{db: db}
}

// Create stores a new Note.
func (r *NoteRepository) Create(ctx context.Context, note *domain.Note) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notes (id, book_id, highlight_id, content, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		note.ID, note.BookID, note.HighlightID, note.Content, note.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: creating note: %w", err)
	}
	return nil
}

// FindByID returns the Note with the given ID, or an error wrapping
// ErrNoteNotFound if it does not exist.
func (r *NoteRepository) FindByID(ctx context.Context, id string) (*domain.Note, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, book_id, highlight_id, content, created_at
		 FROM notes WHERE id = $1`, id)

	note, err := scanNote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: note %q: %w", id, ErrNoteNotFound)
		}
		return nil, fmt.Errorf("postgres: finding note: %w", err)
	}
	return note, nil
}

// ListByBookID returns all Notes belonging to the given Book.
func (r *NoteRepository) ListByBookID(ctx context.Context, bookID string) ([]*domain.Note, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, book_id, highlight_id, content, created_at
		 FROM notes WHERE book_id = $1`, bookID)
	if err != nil {
		return nil, fmt.Errorf("postgres: listing notes: %w", err)
	}
	defer rows.Close()

	notes := make([]*domain.Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scanning note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: listing notes: %w", err)
	}
	return notes, nil
}

// Update persists changes made to an existing Note. It returns an error
// wrapping ErrNoteNotFound if no Note with the given ID exists.
func (r *NoteRepository) Update(ctx context.Context, note *domain.Note) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE notes
		 SET book_id = $2, highlight_id = $3, content = $4, created_at = $5
		 WHERE id = $1`,
		note.ID, note.BookID, note.HighlightID, note.Content, note.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: updating note: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: updating note %q: %w", note.ID, ErrNoteNotFound)
	}
	return nil
}

// Delete removes the Note with the given ID. It returns an error wrapping
// ErrNoteNotFound if no Note with the given ID exists.
func (r *NoteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: deleting note: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: deleting note %q: %w", id, ErrNoteNotFound)
	}
	return nil
}

func scanNote(s rowScanner) (*domain.Note, error) {
	var note domain.Note
	var highlightID sql.NullString
	if err := s.Scan(
		&note.ID, &note.BookID, &highlightID, &note.Content, &note.CreatedAt,
	); err != nil {
		return nil, err
	}
	if highlightID.Valid {
		note.HighlightID = &highlightID.String
	}
	return &note, nil
}
