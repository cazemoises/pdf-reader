// Package postgres_test contains integration tests for the Postgres
// ReadingProgressRepository adapter (ports.ReadingProgressRepository).
//
// These tests run against a real Postgres database - no mocks or fakes for
// the database itself. To run them locally:
//
//  1. Start a Postgres instance with the same credentials the project's
//     docker-compose.yml uses. That file does not publish a host port for
//     the postgres service, so either add a `ports: ["5432:5432"]`
//     override to it, or run a standalone container with matching
//     credentials:
//
//       docker run --rm -d --name pdfreader-postgres \
//         -e POSTGRES_USER=pdfreader -e POSTGRES_PASSWORD=pdfreader -e POSTGRES_DB=pdfreader \
//         -p 5432:5432 postgres:16-alpine
//
//  2. Export DATABASE_URL pointing at it, e.g.:
//
//       export DATABASE_URL="postgres://pdfreader:pdfreader@localhost:5432/pdfreader?sslmode=disable"
//
//  3. Run: go test ./internal/adapters/postgres/...
//
// The schemas in backend/migrations/0001_create_books.sql,
// backend/migrations/0002_create_pages.sql, backend/migrations/0003_create_highlights.sql,
// backend/migrations/0004_create_notes.sql and backend/migrations/0005_create_reading_progress.sql
// are applied by the tests themselves on setup, so no separate migration step is required. If
// DATABASE_URL is unset, all tests in this file are skipped.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/internal/domain"
)

func openTestDBForReadingProgress(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test (see package doc comment for setup instructions)")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("pinging database: %v", err)
	}
	lockSharedTestDB(t, ctx, db)

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("reading migration file %s: %v", migration, err)
		}
		if _, err := db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("applying migration %s: %v", migration, err)
		}
	}

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE reading_progress"); err != nil {
		t.Fatalf("truncating reading_progress table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE notes"); err != nil {
		t.Fatalf("truncating notes table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE highlights CASCADE"); err != nil {
		t.Fatalf("truncating highlights table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE books CASCADE"); err != nil {
		t.Fatalf("truncating books table: %v", err)
	}

	return db
}

func mustCreateTestBookForReadingProgress(t *testing.T, ctx context.Context, db *sql.DB, id string) *domain.Book {
	t.Helper()

	book, err := domain.NewBook(id, "Test Book "+id, "/tmp/"+id+".pdf")
	if err != nil {
		t.Fatalf("building test book: %v", err)
	}

	bookRepo := postgres.NewBookRepository(db)
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatalf("creating test book: %v", err)
	}
	return book
}

func newTestReadingProgress(t *testing.T, bookID string, lastPage int, percentage float64) *domain.ReadingProgress {
	t.Helper()

	progress, err := domain.NewReadingProgress(bookID, lastPage, percentage)
	if err != nil {
		t.Fatalf("building test reading progress: %v", err)
	}
	return progress
}

func TestReadingProgressRepository_SaveThenGetByBookID_RoundTrips(t *testing.T) {
	db := openTestDBForReadingProgress(t)
	ctx := context.Background()
	book := mustCreateTestBookForReadingProgress(t, ctx, db, "book-progress-roundtrip")

	repo := postgres.NewReadingProgressRepository(db)
	want := newTestReadingProgress(t, book.ID, 5, 42.5)

	if err := repo.Save(ctx, want); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	got, err := repo.GetByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("GetByBookID: unexpected error: %v", err)
	}

	if got.BookID != want.BookID {
		t.Errorf("BookID = %q, want %q", got.BookID, want.BookID)
	}
	if got.LastPage != want.LastPage {
		t.Errorf("LastPage = %d, want %d", got.LastPage, want.LastPage)
	}
	if got.Percentage != want.Percentage {
		t.Errorf("Percentage = %v, want %v", got.Percentage, want.Percentage)
	}
	assertTimesClose(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func TestReadingProgressRepository_Save_UpsertOverwritesExisting(t *testing.T) {
	db := openTestDBForReadingProgress(t)
	ctx := context.Background()
	book := mustCreateTestBookForReadingProgress(t, ctx, db, "book-progress-upsert")

	repo := postgres.NewReadingProgressRepository(db)

	first := newTestReadingProgress(t, book.ID, 3, 10)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save (first): unexpected error: %v", err)
	}

	second := newTestReadingProgress(t, book.ID, 20, 88.25)
	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("Save (second): unexpected error: %v", err)
	}

	got, err := repo.GetByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("GetByBookID: unexpected error: %v", err)
	}

	if got.LastPage != second.LastPage {
		t.Errorf("LastPage = %d, want %d", got.LastPage, second.LastPage)
	}
	if got.Percentage != second.Percentage {
		t.Errorf("Percentage = %v, want %v", got.Percentage, second.Percentage)
	}
}

func TestReadingProgressRepository_GetByBookID_NotFoundReturnsErrReadingProgressNotFound(t *testing.T) {
	db := openTestDBForReadingProgress(t)
	ctx := context.Background()

	repo := postgres.NewReadingProgressRepository(db)

	_, err := repo.GetByBookID(ctx, "missing-book-progress")
	if !errors.Is(err, postgres.ErrReadingProgressNotFound) {
		t.Errorf("GetByBookID error = %v, want errors.Is(err, ErrReadingProgressNotFound)", err)
	}
}
