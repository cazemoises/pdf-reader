// Package postgres_test contains integration tests for the Postgres
// BookRepository adapter (ports.BookRepository).
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
// The schema in backend/migrations/0001_create_books.sql is applied by the
// tests themselves on setup, so no separate migration step is required. If
// DATABASE_URL is unset, all tests in this file are skipped.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/internal/domain"
)

func openTestDB(t *testing.T) *sql.DB {
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

	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "0001_create_books.sql"))
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		t.Fatalf("applying migration: %v", err)
	}

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE books CASCADE"); err != nil {
		t.Fatalf("truncating books table: %v", err)
	}

	return db
}

func newTestBook(t *testing.T, id string) *domain.Book {
	t.Helper()

	book, err := domain.NewBook(id, "Test Book "+id, "/tmp/"+id+".pdf")
	if err != nil {
		t.Fatalf("building test book: %v", err)
	}
	return book
}

func assertTimesClose(t *testing.T, label string, got, want time.Time) {
	t.Helper()

	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Millisecond {
		t.Errorf("%s = %v, want %v (diff %v)", label, got, want, diff)
	}
}

func TestBookRepository_CreateThenFindByID_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewBookRepository(db)
	ctx := context.Background()

	want := newTestBook(t, "book-roundtrip")

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}

	if got.ID != want.ID || got.Title != want.Title || got.SourcePath != want.SourcePath || got.Status != want.Status {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
	assertTimesClose(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimesClose(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func TestBookRepository_FindByID_NonExistentReturnsError(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewBookRepository(db)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("FindByID: expected error for non-existent id, got nil")
	}
	if !errors.Is(err, postgres.ErrBookNotFound) {
		t.Errorf("FindByID: error = %v, want wrapping postgres.ErrBookNotFound", err)
	}
}

func TestBookRepository_Update_PersistsChanges(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewBookRepository(db)
	ctx := context.Background()

	book := newTestBook(t, "book-update")
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if err := book.UpdateStatus(domain.BookStatusReady); err != nil {
		t.Fatalf("UpdateStatus: unexpected error: %v", err)
	}
	book.Title = "Updated Title"

	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if got.Status != domain.BookStatusReady {
		t.Errorf("Status = %q, want %q", got.Status, domain.BookStatusReady)
	}
	if got.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated Title")
	}
}

func TestBookRepository_List_ReturnsAllCreatedBooks(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewBookRepository(db)
	ctx := context.Background()

	book1 := newTestBook(t, "book-list-1")
	book2 := newTestBook(t, "book-list-2")
	if err := repo.Create(ctx, book1); err != nil {
		t.Fatalf("Create book1: unexpected error: %v", err)
	}
	if err := repo.Create(ctx, book2); err != nil {
		t.Fatalf("Create book2: unexpected error: %v", err)
	}

	books, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("len(books) = %d, want 2", len(books))
	}

	ids := map[string]bool{}
	for _, b := range books {
		ids[b.ID] = true
	}
	if !ids[book1.ID] || !ids[book2.ID] {
		t.Errorf("List() ids = %v, want to contain %q and %q", ids, book1.ID, book2.ID)
	}
}

func TestBookRepository_Delete_RemovesBook(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewBookRepository(db)
	ctx := context.Background()

	book := newTestBook(t, "book-delete")
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if err := repo.Delete(ctx, book.ID); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	if _, err := repo.FindByID(ctx, book.ID); err == nil {
		t.Fatal("FindByID after Delete: expected error, got nil")
	}
}
