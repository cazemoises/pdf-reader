// Package postgres_test contains integration tests for the Postgres
// NoteRepository adapter (ports.NoteRepository).
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
// backend/migrations/0002_create_pages.sql, backend/migrations/0003_create_highlights.sql
// and backend/migrations/0004_create_notes.sql are applied by the tests
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

func openTestDBForNotes(t *testing.T) *sql.DB {
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

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0006_highlight_char_offsets.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("reading migration file %s: %v", migration, err)
		}
		if _, err := db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("applying migration %s: %v", migration, err)
		}
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

func mustCreateTestBookForNotes(t *testing.T, ctx context.Context, db *sql.DB, id string) *domain.Book {
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

func mustCreateTestHighlightForNotes(t *testing.T, ctx context.Context, db *sql.DB, id, bookID string) *domain.Highlight {
	t.Helper()

	charRange := domain.CharRange{Start: 10, End: 40}
	highlight, err := domain.NewHighlight(id, bookID, 1, charRange, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}

	highlightRepo := postgres.NewHighlightRepository(db)
	if err := highlightRepo.Create(ctx, highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}

func newTestNote(t *testing.T, id, bookID string, highlightID *string, content string) *domain.Note {
	t.Helper()

	note, err := domain.NewNote(id, bookID, highlightID, content)
	if err != nil {
		t.Fatalf("building test note: %v", err)
	}
	return note
}

func TestNoteRepository_CreateThenFindByID_RoundTrips_NilHighlightID(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-roundtrip-nil")

	repo := postgres.NewNoteRepository(db)
	want := newTestNote(t, "note-roundtrip-nil", book.ID, nil, "a note with no highlight")

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}

	if got.ID != want.ID || got.BookID != want.BookID || got.Content != want.Content {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
	if got.HighlightID != nil {
		t.Errorf("HighlightID = %v, want nil", *got.HighlightID)
	}
	assertTimesClose(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
}

func TestNoteRepository_CreateThenFindByID_RoundTrips_WithHighlightID(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-roundtrip-highlight")
	highlight := mustCreateTestHighlightForNotes(t, ctx, db, "highlight-note-roundtrip", book.ID)

	repo := postgres.NewNoteRepository(db)
	want := newTestNote(t, "note-roundtrip-highlight", book.ID, &highlight.ID, "a note tied to a highlight")

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}

	if got.ID != want.ID || got.BookID != want.BookID || got.Content != want.Content {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
	if got.HighlightID == nil || *got.HighlightID != highlight.ID {
		t.Errorf("HighlightID = %v, want %q", got.HighlightID, highlight.ID)
	}
	assertTimesClose(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
}

func TestNoteRepository_FindByID_NotFoundReturnsErrNoteNotFound(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()

	repo := postgres.NewNoteRepository(db)

	_, err := repo.FindByID(ctx, "missing-note")
	if !errors.Is(err, postgres.ErrNoteNotFound) {
		t.Errorf("FindByID error = %v, want errors.Is(err, ErrNoteNotFound)", err)
	}
}

func TestNoteRepository_ListByBookID_ReturnsOnlyNotesForThatBook(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	bookA := mustCreateTestBookForNotes(t, ctx, db, "book-note-a")
	bookB := mustCreateTestBookForNotes(t, ctx, db, "book-note-b")

	repo := postgres.NewNoteRepository(db)

	n1 := newTestNote(t, "note-a1", bookA.ID, nil, "note a1")
	n2 := newTestNote(t, "note-a2", bookA.ID, nil, "note a2")
	n3 := newTestNote(t, "note-b1", bookB.ID, nil, "note b1")

	for _, n := range []*domain.Note{n1, n2, n3} {
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("Create note %s: unexpected error: %v", n.ID, err)
		}
	}

	notes, err := repo.ListByBookID(ctx, bookA.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}

	if len(notes) != 2 {
		t.Fatalf("len(notes) = %d, want 2", len(notes))
	}
	for _, n := range notes {
		if n.BookID != bookA.ID {
			t.Errorf("note %s has BookID = %s, want %s", n.ID, n.BookID, bookA.ID)
		}
	}
}

func TestNoteRepository_ListByBookID_NoNotesReturnsEmptySlice(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-empty")

	repo := postgres.NewNoteRepository(db)

	notes, err := repo.ListByBookID(ctx, book.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("len(notes) = %d, want 0", len(notes))
	}
}

func TestNoteRepository_Update_PersistsChanges(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-update")

	repo := postgres.NewNoteRepository(db)
	note := newTestNote(t, "note-update", book.ID, nil, "original content")

	if err := repo.Create(ctx, note); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	note.Content = "updated content"
	if err := repo.Update(ctx, note); err != nil {
		t.Fatalf("Update: unexpected error: %v", err)
	}

	got, err := repo.FindByID(ctx, note.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want %q", got.Content, "updated content")
	}
}

func TestNoteRepository_Update_NotFoundReturnsErrNoteNotFound(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-update-missing")

	repo := postgres.NewNoteRepository(db)
	note := newTestNote(t, "missing-note-update", book.ID, nil, "content")

	err := repo.Update(ctx, note)
	if !errors.Is(err, postgres.ErrNoteNotFound) {
		t.Errorf("Update error = %v, want errors.Is(err, ErrNoteNotFound)", err)
	}
}

func TestNoteRepository_Delete_RemovesNote(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()
	book := mustCreateTestBookForNotes(t, ctx, db, "book-note-delete")

	repo := postgres.NewNoteRepository(db)
	note := newTestNote(t, "note-delete", book.ID, nil, "content")

	if err := repo.Create(ctx, note); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if err := repo.Delete(ctx, note.ID); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	_, err := repo.FindByID(ctx, note.ID)
	if !errors.Is(err, postgres.ErrNoteNotFound) {
		t.Errorf("FindByID after delete error = %v, want errors.Is(err, ErrNoteNotFound)", err)
	}
}

func TestNoteRepository_Delete_NotFoundReturnsErrNoteNotFound(t *testing.T) {
	db := openTestDBForNotes(t)
	ctx := context.Background()

	repo := postgres.NewNoteRepository(db)

	err := repo.Delete(ctx, "missing-note-delete")
	if !errors.Is(err, postgres.ErrNoteNotFound) {
		t.Errorf("Delete error = %v, want errors.Is(err, ErrNoteNotFound)", err)
	}
}
