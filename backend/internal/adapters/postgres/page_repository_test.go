// Package postgres_test contains integration tests for the Postgres
// PageRepository adapter (ports.PageRepository).
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
// The schemas in backend/migrations/0001_create_books.sql and
// backend/migrations/0002_create_pages.sql are applied by the tests
// themselves on setup, so no separate migration step is required. If
// DATABASE_URL is unset, all tests in this file are skipped.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/internal/domain"
)

func openTestDBForPages(t *testing.T) *sql.DB {
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

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("reading migration file %s: %v", migration, err)
		}
		if _, err := db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("applying migration %s: %v", migration, err)
		}
	}

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE pages"); err != nil {
		t.Fatalf("truncating pages table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE books CASCADE"); err != nil {
		t.Fatalf("truncating books table: %v", err)
	}

	return db
}

func mustCreateTestBook(t *testing.T, ctx context.Context, db *sql.DB, id string) *domain.Book {
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

func newTestPage(t *testing.T, bookID string, number int) *domain.Page {
	t.Helper()

	page, err := domain.NewPage(bookID, number, "page text", 612.0, 792.0)
	if err != nil {
		t.Fatalf("building test page: %v", err)
	}
	return page
}

func TestPageRepository_CreateThenListByBookID_RoundTrips(t *testing.T) {
	db := openTestDBForPages(t)
	ctx := context.Background()
	book := mustCreateTestBook(t, ctx, db, "book-page-roundtrip")

	repo := postgres.NewPageRepository(db)
	want := newTestPage(t, book.ID, 1)

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	pages, err := repo.ListByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("len(pages) = %d, want 1", len(pages))
	}
	got := pages[0]
	if got.BookID != want.BookID || got.Number != want.Number || got.Text != want.Text ||
		got.Width != want.Width || got.Height != want.Height {
		t.Errorf("ListByBookID = %+v, want %+v", got, want)
	}
}

func TestPageRepository_ListByBookID_ReturnsAllOrderedByNumber(t *testing.T) {
	db := openTestDBForPages(t)
	ctx := context.Background()
	book := mustCreateTestBook(t, ctx, db, "book-page-ordering")

	repo := postgres.NewPageRepository(db)

	page3 := newTestPage(t, book.ID, 3)
	page1 := newTestPage(t, book.ID, 1)
	page2 := newTestPage(t, book.ID, 2)

	for _, p := range []*domain.Page{page3, page1, page2} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create page %d: unexpected error: %v", p.Number, err)
		}
	}

	pages, err := repo.ListByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}

	if len(pages) != 3 {
		t.Fatalf("len(pages) = %d, want 3", len(pages))
	}
	for i, want := range []int{1, 2, 3} {
		if pages[i].Number != want {
			t.Errorf("pages[%d].Number = %d, want %d", i, pages[i].Number, want)
		}
	}
}

func TestPageRepository_ListByBookID_NoPagesReturnsEmptySlice(t *testing.T) {
	db := openTestDBForPages(t)
	ctx := context.Background()
	book := mustCreateTestBook(t, ctx, db, "book-page-empty")

	repo := postgres.NewPageRepository(db)

	pages, err := repo.ListByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("len(pages) = %d, want 0", len(pages))
	}
}
