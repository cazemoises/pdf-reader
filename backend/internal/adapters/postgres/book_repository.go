// Package postgres implements ports.BookRepository backed by a Postgres
// database via the standard library database/sql package.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// ErrBookNotFound is returned (wrapped) when a Book lookup, update, or
// delete targets an id that does not exist in the books table.
var ErrBookNotFound = errors.New("postgres: book not found")

// BookRepository implements ports.BookRepository against a books table in
// Postgres.
type BookRepository struct {
	db *sql.DB
}

// NewBookRepository creates a BookRepository using db as its connection
// pool. db is not owned by the repository; callers remain responsible for
// closing it.
func NewBookRepository(db *sql.DB) *BookRepository {
	return &BookRepository{db: db}
}

// Create stores a new Book.
func (r *BookRepository) Create(ctx context.Context, book *domain.Book) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO books (id, title, source_path, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		book.ID, book.Title, book.SourcePath, string(book.Status), book.CreatedAt, book.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: creating book: %w", err)
	}
	return nil
}

// FindByID returns the Book with the given ID, or an error wrapping
// ErrBookNotFound if it does not exist.
func (r *BookRepository) FindByID(ctx context.Context, id string) (*domain.Book, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, title, source_path, status, created_at, updated_at
		 FROM books WHERE id = $1`, id)

	book, err := scanBook(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: book %q: %w", id, ErrBookNotFound)
		}
		return nil, fmt.Errorf("postgres: finding book: %w", err)
	}
	return book, nil
}

// Update persists changes made to an existing Book. It returns an error
// wrapping ErrBookNotFound if no Book with the given ID exists.
func (r *BookRepository) Update(ctx context.Context, book *domain.Book) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE books SET title = $2, source_path = $3, status = $4, created_at = $5, updated_at = $6
		 WHERE id = $1`,
		book.ID, book.Title, book.SourcePath, string(book.Status), book.CreatedAt, book.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: updating book: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: updating book %q: %w", book.ID, ErrBookNotFound)
	}
	return nil
}

// List returns all known Books.
func (r *BookRepository) List(ctx context.Context) ([]*domain.Book, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, source_path, status, created_at, updated_at
		 FROM books ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: listing books: %w", err)
	}
	defer rows.Close()

	var books []*domain.Book
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scanning book: %w", err)
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: listing books: %w", err)
	}
	return books, nil
}

// Delete removes the Book with the given ID. It returns an error wrapping
// ErrBookNotFound if no Book with the given ID exists.
func (r *BookRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM books WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: deleting book: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: deleting book %q: %w", id, ErrBookNotFound)
	}
	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBook(s rowScanner) (*domain.Book, error) {
	var (
		book   domain.Book
		status string
	)
	if err := s.Scan(&book.ID, &book.Title, &book.SourcePath, &status, &book.CreatedAt, &book.UpdatedAt); err != nil {
		return nil, err
	}
	book.Status = domain.BookStatus(status)
	return &book, nil
}
