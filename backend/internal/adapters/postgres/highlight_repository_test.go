// Package postgres_test contains integration tests for the Postgres
// HighlightRepository adapter (ports.HighlightRepository).
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
// backend/migrations/0002_create_pages.sql and
// backend/migrations/0003_create_highlights.sql are applied by the tests
// themselves on setup, so no separate migration step is required. If
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

func openTestDBForHighlights(t *testing.T) *sql.DB {
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

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("reading migration file %s: %v", migration, err)
		}
		if _, err := db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("applying migration %s: %v", migration, err)
		}
	}

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE highlights"); err != nil {
		t.Fatalf("truncating highlights table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE books CASCADE"); err != nil {
		t.Fatalf("truncating books table: %v", err)
	}

	return db
}

func mustCreateTestBookForHighlights(t *testing.T, ctx context.Context, db *sql.DB, id string) *domain.Book {
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

func newTestHighlight(t *testing.T, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	box := domain.BoundingBox{X: 10, Y: 20, Width: 100, Height: 30}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, box, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	return highlight
}

func TestHighlightRepository_CreateThenFindByID_RoundTrips(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	book := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-roundtrip")

	repo := postgres.NewHighlightRepository(db)
	want := newTestHighlight(t, "highlight-roundtrip", book.ID, 1)

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}

	if got.ID != want.ID || got.BookID != want.BookID || got.PageNumber != want.PageNumber ||
		got.Box != want.Box || got.Color != want.Color || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
}

func TestHighlightRepository_FindByID_NotFoundReturnsErrHighlightNotFound(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()

	repo := postgres.NewHighlightRepository(db)

	_, err := repo.FindByID(ctx, "missing-highlight")
	if !errors.Is(err, postgres.ErrHighlightNotFound) {
		t.Errorf("FindByID error = %v, want errors.Is(err, ErrHighlightNotFound)", err)
	}
}

func TestHighlightRepository_ListByBookID_ReturnsOnlyHighlightsForThatBook(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	bookA := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-a")
	bookB := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-b")

	repo := postgres.NewHighlightRepository(db)

	h1 := newTestHighlight(t, "highlight-a1", bookA.ID, 1)
	h2 := newTestHighlight(t, "highlight-a2", bookA.ID, 2)
	h3 := newTestHighlight(t, "highlight-b1", bookB.ID, 1)

	for _, h := range []*domain.Highlight{h1, h2, h3} {
		if err := repo.Create(ctx, h); err != nil {
			t.Fatalf("Create highlight %s: unexpected error: %v", h.ID, err)
		}
	}

	highlights, err := repo.ListByBookID(ctx, bookA.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}

	if len(highlights) != 2 {
		t.Fatalf("len(highlights) = %d, want 2", len(highlights))
	}
	for _, h := range highlights {
		if h.BookID != bookA.ID {
			t.Errorf("highlight %s has BookID = %s, want %s", h.ID, h.BookID, bookA.ID)
		}
	}
}

func TestHighlightRepository_ListByBookID_NoHighlightsReturnsEmptySlice(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	book := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-empty")

	repo := postgres.NewHighlightRepository(db)

	highlights, err := repo.ListByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}
	if len(highlights) != 0 {
		t.Errorf("len(highlights) = %d, want 0", len(highlights))
	}
}

func TestHighlightRepository_Update_PersistsChanges(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	book := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-update")

	repo := postgres.NewHighlightRepository(db)
	highlight := newTestHighlight(t, "highlight-update", book.ID, 1)

	if err := repo.Create(ctx, highlight); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	highlight.Color = "green"
	if err := repo.Update(ctx, highlight); err != nil {
		t.Fatalf("Update: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, highlight.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if got.Color != "green" {
		t.Errorf("Color = %q, want %q", got.Color, "green")
	}
}

func TestHighlightRepository_Update_NotFoundReturnsErrHighlightNotFound(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	book := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-update-missing")

	repo := postgres.NewHighlightRepository(db)
	highlight := newTestHighlight(t, "missing-highlight-update", book.ID, 1)

	err := repo.Update(ctx, highlight)
	if !errors.Is(err, postgres.ErrHighlightNotFound) {
		t.Errorf("Update error = %v, want errors.Is(err, ErrHighlightNotFound)", err)
	}
}

func TestHighlightRepository_Delete_RemovesHighlight(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()
	book := mustCreateTestBookForHighlights(t, ctx, db, "book-highlight-delete")

	repo := postgres.NewHighlightRepository(db)
	highlight := newTestHighlight(t, "highlight-delete", book.ID, 1)

	if err := repo.Create(ctx, highlight); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if err := repo.Delete(ctx, highlight.ID); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	_, err := repo.FindByID(ctx, highlight.ID)
	if !errors.Is(err, postgres.ErrHighlightNotFound) {
		t.Errorf("FindByID after delete error = %v, want errors.Is(err, ErrHighlightNotFound)", err)
	}
}

func TestHighlightRepository_Delete_NotFoundReturnsErrHighlightNotFound(t *testing.T) {
	db := openTestDBForHighlights(t)
	ctx := context.Background()

	repo := postgres.NewHighlightRepository(db)

	err := repo.Delete(ctx, "missing-highlight-delete")
	if !errors.Is(err, postgres.ErrHighlightNotFound) {
		t.Errorf("Delete error = %v, want errors.Is(err, ErrHighlightNotFound)", err)
	}
}
