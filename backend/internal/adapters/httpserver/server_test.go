// Package httpserver_test contains integration tests for the HTTP adapter
// (internal/adapters/httpserver.Server). These tests run against a real
// Postgres database and a real HTTPTextExtractor pointed at a fake extractor
// HTTP server - no mocks or fakes for the database itself. To run them
// locally:
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
//  3. Run: go test ./internal/adapters/httpserver/...
//
// The schemas in backend/migrations/0001_create_books.sql,
// backend/migrations/0002_create_pages.sql and
// backend/migrations/0003_create_highlights.sql are applied by the tests
// themselves on setup, so no separate migration step is required. If
// DATABASE_URL is unset, all tests in this file are skipped.
package httpserver_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/filestorage"
	"pdf-reader/backend/internal/adapters/httpextractor"
	"pdf-reader/backend/internal/adapters/httpserver"
	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/internal/domain"
	"pdf-reader/backend/internal/ports"
)

// sharedTestDBLockKey identifies a Postgres session-level advisory lock
// used to serialize this package's integration tests against tests in
// other packages (e.g. postgres_test) that point at the same database and
// TRUNCATE the same tables. `go test ./...` runs different packages' test
// binaries concurrently by default; without this lock, one package
// truncating a table while another package's test is mid-flight against
// that same table causes real Postgres deadlocks and silently lost rows.
// This numeric key must match the one duplicated in postgres_test for the
// lock to actually coordinate across packages.
const sharedTestDBLockKey int64 = 78412093651

// lockSharedTestDB acquires sharedTestDBLockKey and holds it for the
// lifetime of the test. It pins db to a single pooled connection first, so
// the advisory lock (which is tied to the connection that took it) can't
// be silently dropped by the pool checking that connection back in.
func lockSharedTestDB(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", sharedTestDBLockKey); err != nil {
		t.Fatalf("acquiring shared test db lock: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", sharedTestDBLockKey); err != nil {
			t.Logf("releasing shared test db lock: %v", err)
		}
	})
}

func openTestDBForServer(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping httpserver integration test (see package doc comment for setup instructions)")
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

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql", "0006_highlight_char_offsets.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("reading migration file %s: %v", migration, err)
		}
		if _, err := db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("applying migration %s: %v", migration, err)
		}
	}

	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE notes CASCADE"); err != nil {
		t.Fatalf("truncating notes table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE reading_progress"); err != nil {
		t.Fatalf("truncating reading_progress table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE highlights CASCADE"); err != nil {
		t.Fatalf("truncating highlights table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE pages"); err != nil {
		t.Fatalf("truncating pages table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE books CASCADE"); err != nil {
		t.Fatalf("truncating books table: %v", err)
	}

	return db
}

// fakeExtractorServer stands in for the real Python extractor service so
// these tests stay hermetic. It always responds with the given status code
// and body for POST /extract.
func fakeExtractorServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type testDeps struct {
	bookRepo      ports.BookRepository
	pageRepo      ports.PageRepository
	highlightRepo ports.HighlightRepository
	noteRepo      ports.NoteRepository
	progressRepo  ports.ReadingProgressRepository
}

// newTestServer wires a real Server against the given db and a fake
// extractor server, returning an httptest.Server exposing it over HTTP.
func newTestServer(t *testing.T, db *sql.DB, extractorURL string) (*httptest.Server, testDeps) {
	t.Helper()

	deps := testDeps{
		bookRepo:      postgres.NewBookRepository(db),
		pageRepo:      postgres.NewPageRepository(db),
		highlightRepo: postgres.NewHighlightRepository(db),
		noteRepo:      postgres.NewNoteRepository(db),
		progressRepo:  postgres.NewReadingProgressRepository(db),
	}
	extractor := httpextractor.NewHTTPTextExtractor(extractorURL, nil)
	storage := filestorage.NewFileSystemStorage(t.TempDir())

	server := httpserver.NewServer(deps.bookRepo, deps.pageRepo, deps.highlightRepo, deps.noteRepo, deps.progressRepo, extractor, storage)
	httpSrv := httptest.NewServer(server)
	t.Cleanup(httpSrv.Close)

	return httpSrv, deps
}

// multipartBody builds a multipart/form-data body with a text field and a
// file field, returning the body reader and its Content-Type header value.
func multipartBody(t *testing.T, fieldName, fieldValue, fileFieldName, filename, fileContent string) (io.Reader, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField(fieldName, fieldValue); err != nil {
		t.Fatalf("writing field %s: %v", fieldName, err)
	}

	part, err := writer.CreateFormFile(fileFieldName, filename)
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write([]byte(fileContent)); err != nil {
		t.Fatalf("writing file content: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	return body, writer.FormDataContentType()
}

// bookJSON mirrors domain.Book's camelCase json tags.
type bookJSON struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SourcePath string `json:"sourcePath"`
	Status     string `json:"status"`
}

func TestPostBooks_CreatesBookExtractsPagesAndReturnsCreated(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{
		"pages": [
			{"page_number": 1, "width": 612, "height": 792, "blocks": [{"text": "hello"}]},
			{"page_number": 2, "width": 612, "height": 792, "blocks": [{"text": "world"}]}
		]
	}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)

	body, contentType := multipartBody(t, "title", "My Book", "file", "book.pdf", "fake pdf bytes")

	resp, err := http.Post(httpSrv.URL+"/books", contentType, body)
	if err != nil {
		t.Fatalf("POST /books: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Error("response ID is empty, want a generated id")
	}
	if got.Title != "My Book" {
		t.Errorf("Title = %q, want %q", got.Title, "My Book")
	}
	if got.Status != string(domain.BookStatusReady) {
		t.Errorf("Status = %q, want %q", got.Status, domain.BookStatusReady)
	}

	ctx := context.Background()
	storedBook, err := deps.bookRepo.FindByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if storedBook.Status != domain.BookStatusReady {
		t.Errorf("stored book status = %q, want %q", storedBook.Status, domain.BookStatusReady)
	}

	pages, err := deps.pageRepo.ListByBookID(ctx, got.ID)
	if err != nil {
		t.Fatalf("ListByBookID: unexpected error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}
	if pages[0].Text != "hello" || pages[1].Text != "world" {
		t.Errorf("pages text = %q, %q, want %q, %q", pages[0].Text, pages[1].Text, "hello", "world")
	}
}

func TestPostBooks_ExtractionFailure_MarksBookFailedAndReturnsError(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusInternalServerError, `boom`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)

	body, contentType := multipartBody(t, "title", "Doomed Book", "file", "book.pdf", "fake pdf bytes")

	resp, err := http.Post(httpSrv.URL+"/books", contentType, body)
	if err != nil {
		t.Fatalf("POST /books: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}

	var got bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Fatal("response ID is empty, want a generated id")
	}

	storedBook, err := deps.bookRepo.FindByID(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if storedBook.Status != domain.BookStatusFailed {
		t.Errorf("stored book status = %q, want %q", storedBook.Status, domain.BookStatusFailed)
	}
}

func mustCreateTestBookDirect(t *testing.T, deps testDeps, id string) *domain.Book {
	t.Helper()

	book, err := domain.NewBook(id, "Test Book "+id, "/tmp/"+id+".pdf")
	if err != nil {
		t.Fatalf("building test book: %v", err)
	}
	if err := deps.bookRepo.Create(context.Background(), book); err != nil {
		t.Fatalf("creating test book: %v", err)
	}
	return book
}

func TestGetBook_ReturnsBook(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-get-found")

	resp, err := http.Get(httpSrv.URL + "/books/" + book.ID)
	if err != nil {
		t.Fatalf("GET /books/{id}: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != book.ID {
		t.Errorf("ID = %q, want %q", got.ID, book.ID)
	}
	if got.Title != book.Title {
		t.Errorf("Title = %q, want %q", got.Title, book.Title)
	}
}

func TestGetBooks_ListsAllBooks(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	mustCreateTestBookDirect(t, deps, "book-list-a")
	mustCreateTestBookDirect(t, deps, "book-list-b")

	resp, err := http.Get(httpSrv.URL + "/books")
	if err != nil {
		t.Fatalf("GET /books: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got []bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestGetBooks_EmptyWhenNoneCreated(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp, err := http.Get(httpSrv.URL + "/books")
	if err != nil {
		t.Fatalf("GET /books: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	if got := strings.TrimSpace(string(rawBody)); got != "[]" {
		t.Errorf("response body = %q, want %q (nil slices marshal to JSON null, breaking clients that expect an array)", got, "[]")
	}

	var got []bookJSON
	if err := json.Unmarshal(rawBody, &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestGetBookFile_ReturnsFileBytes(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)

	body, contentType := multipartBody(t, "title", "File Book", "file", "book.pdf", "fake pdf bytes")
	resp, err := http.Post(httpSrv.URL+"/books", contentType, body)
	if err != nil {
		t.Fatalf("POST /books: unexpected error: %v", err)
	}
	defer resp.Body.Close()
	var created bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	_ = deps

	fileResp, err := http.Get(httpSrv.URL + "/books/" + created.ID + "/file")
	if err != nil {
		t.Fatalf("GET /books/{id}/file: unexpected error: %v", err)
	}
	defer fileResp.Body.Close()

	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", fileResp.StatusCode, http.StatusOK)
	}
	if ct := fileResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/pdf")
	}

	gotBytes, err := io.ReadAll(fileResp.Body)
	if err != nil {
		t.Fatalf("reading file response body: %v", err)
	}
	if string(gotBytes) != "fake pdf bytes" {
		t.Errorf("body = %q, want %q", string(gotBytes), "fake pdf bytes")
	}
}

func TestGetBookFile_NotFoundReturns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp, err := http.Get(httpSrv.URL + "/books/does-not-exist/file")
	if err != nil {
		t.Fatalf("GET /books/{id}/file: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestGetBook_NotFoundReturns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp, err := http.Get(httpSrv.URL + "/books/does-not-exist")
	if err != nil {
		t.Fatalf("GET /books/{id}: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// pageJSON mirrors domain.Page's camelCase json tags.
type pageJSON struct {
	BookID string  `json:"bookId"`
	Number int     `json:"number"`
	Text   string  `json:"text"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func mustCreateTestPageDirect(t *testing.T, deps testDeps, bookID string, number int) *domain.Page {
	t.Helper()

	page, err := domain.NewPage(bookID, number, "page text", 612, 792)
	if err != nil {
		t.Fatalf("building test page: %v", err)
	}
	if err := deps.pageRepo.Create(context.Background(), page); err != nil {
		t.Fatalf("creating test page: %v", err)
	}
	return page
}

func TestGetPage_ReturnsPage(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-page-get-found")
	mustCreateTestPageDirect(t, deps, book.ID, 1)

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/pages/1", httpSrv.URL, book.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/pages/{number}: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got pageJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Number != 1 || got.Text != "page text" {
		t.Errorf("got = %+v, want Number=1, Text=%q", got, "page text")
	}
}

func TestGetPage_NotFoundReturns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-page-get-missing")

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/pages/99", httpSrv.URL, book.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/pages/{number}: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// rangeJSON mirrors domain.CharRange's camelCase json tags.
type rangeJSON struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// highlightJSON mirrors domain.Highlight's camelCase json tags.
type highlightJSON struct {
	ID         string    `json:"id"`
	BookID     string    `json:"bookId"`
	PageNumber int       `json:"pageNumber"`
	Range      rangeJSON `json:"range"`
	Color      string    `json:"color"`
}

func TestPostHighlights_CreatesHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-highlight-post")

	reqBody := `{"pageNumber": 1, "range": {"start": 10, "end": 40}, "color": "yellow"}`

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/highlights: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got highlightJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Error("response ID is empty, want a generated id")
	}
	if got.BookID != book.ID || got.PageNumber != 1 || got.Color != "yellow" {
		t.Errorf("got = %+v, want BookID=%q, PageNumber=1, Color=yellow", got, book.ID)
	}
	if got.Range != (rangeJSON{Start: 10, End: 40}) {
		t.Errorf("Range = %+v, want {10 40}", got.Range)
	}

	stored, err := deps.highlightRepo.FindByID(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if stored.BookID != book.ID {
		t.Errorf("stored highlight BookID = %q, want %q", stored.BookID, book.ID)
	}
}

func mustCreateTestHighlightDirect(t *testing.T, deps testDeps, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	charRange := domain.CharRange{Start: 1, End: 4}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, charRange, "green")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	if err := deps.highlightRepo.Create(context.Background(), highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}

func TestGetHighlights_ListsHighlightsForBook(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	bookA := mustCreateTestBookDirect(t, deps, "book-highlight-list-a")
	bookB := mustCreateTestBookDirect(t, deps, "book-highlight-list-b")
	mustCreateTestHighlightDirect(t, deps, "highlight-list-a1", bookA.ID, 1)
	mustCreateTestHighlightDirect(t, deps, "highlight-list-a2", bookA.ID, 2)
	mustCreateTestHighlightDirect(t, deps, "highlight-list-b1", bookB.ID, 1)

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, bookA.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/highlights: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got []highlightJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, h := range got {
		if h.BookID != bookA.ID {
			t.Errorf("highlight %s has BookID = %q, want %q", h.ID, h.BookID, bookA.ID)
		}
	}
}

// noteJSON mirrors domain.Note's camelCase json tags.
type noteJSON struct {
	ID          string  `json:"id"`
	BookID      string  `json:"bookId"`
	HighlightID *string `json:"highlightId,omitempty"`
	Content     string  `json:"content"`
}

func TestPostNotes_CreatesNoteWithoutHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-note-post")

	reqBody := `{"content": "an important thought"}`

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got noteJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Error("response ID is empty, want a generated id")
	}
	if got.BookID != book.ID || got.Content != "an important thought" {
		t.Errorf("got = %+v, want BookID=%q, Content=%q", got, book.ID, "an important thought")
	}
	if got.HighlightID != nil {
		t.Errorf("HighlightID = %v, want nil", *got.HighlightID)
	}

	stored, err := deps.noteRepo.FindByID(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if stored.BookID != book.ID {
		t.Errorf("stored note BookID = %q, want %q", stored.BookID, book.ID)
	}
}

func TestPostNotes_CreatesNoteWithHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-note-post-with-highlight")
	highlight := mustCreateTestHighlightDirect(t, deps, "highlight-note-post", book.ID, 1)

	reqBody := fmt.Sprintf(`{"content": "on this highlight", "highlightId": %q}`, highlight.ID)

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got noteJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.HighlightID == nil || *got.HighlightID != highlight.ID {
		t.Errorf("HighlightID = %v, want %q", got.HighlightID, highlight.ID)
	}
}

func TestPostNotes_BookNotFound_Returns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	reqBody := `{"content": "orphan note"}`

	resp, err := http.Post(httpSrv.URL+"/books/does-not-exist/notes", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestPostNotes_HighlightFromAnotherBook_Returns400(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	bookA := mustCreateTestBookDirect(t, deps, "book-note-post-mismatch-a")
	bookB := mustCreateTestBookDirect(t, deps, "book-note-post-mismatch-b")
	highlightOnB := mustCreateTestHighlightDirect(t, deps, "highlight-note-post-mismatch", bookB.ID, 1)

	reqBody := fmt.Sprintf(`{"content": "wrong book highlight", "highlightId": %q}`, highlightOnB.ID)

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, bookA.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestPostNotes_HighlightNotFound_Returns400(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-note-post-missing-highlight")

	reqBody := `{"content": "dangling highlight", "highlightId": "does-not-exist"}`

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func mustCreateTestNoteDirect(t *testing.T, deps testDeps, id, bookID string, highlightID *string) *domain.Note {
	t.Helper()

	note, err := domain.NewNote(id, bookID, highlightID, "note content "+id)
	if err != nil {
		t.Fatalf("building test note: %v", err)
	}
	if err := deps.noteRepo.Create(context.Background(), note); err != nil {
		t.Fatalf("creating test note: %v", err)
	}
	return note
}

func TestGetNotes_ListsNotesForBook(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	bookA := mustCreateTestBookDirect(t, deps, "book-note-list-a")
	bookB := mustCreateTestBookDirect(t, deps, "book-note-list-b")
	mustCreateTestNoteDirect(t, deps, "note-list-a1", bookA.ID, nil)
	mustCreateTestNoteDirect(t, deps, "note-list-a2", bookA.ID, nil)
	mustCreateTestNoteDirect(t, deps, "note-list-b1", bookB.ID, nil)

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, bookA.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got []noteJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, n := range got {
		if n.BookID != bookA.ID {
			t.Errorf("note %s has BookID = %q, want %q", n.ID, n.BookID, bookA.ID)
		}
	}
}

func TestGetNotes_EmptyWhenNoneCreated(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-note-list-empty")

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, book.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got []noteJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestGetNotes_BookNotFound_Returns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp, err := http.Get(httpSrv.URL + "/books/does-not-exist/notes")
	if err != nil {
		t.Fatalf("GET /books/{id}/notes: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// progressJSON mirrors domain.ReadingProgress's camelCase json tags.
type progressJSON struct {
	BookID     string  `json:"bookId"`
	LastPage   int     `json:"lastPage"`
	Percentage float64 `json:"percentage"`
}

func mustCreateTestProgressDirect(t *testing.T, deps testDeps, bookID string, lastPage int, percentage float64) *domain.ReadingProgress {
	t.Helper()

	progress, err := domain.NewReadingProgress(bookID, lastPage, percentage)
	if err != nil {
		t.Fatalf("building test reading progress: %v", err)
	}
	if err := deps.progressRepo.Save(context.Background(), progress); err != nil {
		t.Fatalf("saving test reading progress: %v", err)
	}
	return progress
}

func TestGetProgress_ReturnsProgress(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-progress-get-found")
	mustCreateTestProgressDirect(t, deps, book.ID, 5, 42.5)

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, book.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/progress: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got progressJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.BookID != book.ID || got.LastPage != 5 || got.Percentage != 42.5 {
		t.Errorf("got = %+v, want BookID=%q, LastPage=5, Percentage=42.5", got, book.ID)
	}
}

func TestGetProgress_NoProgressSaved_Returns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-progress-get-missing")

	resp, err := http.Get(fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, book.ID))
	if err != nil {
		t.Fatalf("GET /books/{id}/progress: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestGetProgress_BookNotFound_Returns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp, err := http.Get(httpSrv.URL + "/books/does-not-exist/progress")
	if err != nil {
		t.Fatalf("GET /books/{id}/progress: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func putRequest(t *testing.T, url, body string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: unexpected error: %v", url, err)
	}
	return resp
}

func TestPutProgress_CreatesProgress(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-progress-put-create")

	resp := putRequest(t, fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, book.ID), `{"lastPage": 3, "percentage": 20}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got progressJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.BookID != book.ID || got.LastPage != 3 || got.Percentage != 20 {
		t.Errorf("got = %+v, want BookID=%q, LastPage=3, Percentage=20", got, book.ID)
	}

	stored, err := deps.progressRepo.GetByBookID(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("GetByBookID: unexpected error: %v", err)
	}
	if stored.LastPage != 3 || stored.Percentage != 20 {
		t.Errorf("stored progress = %+v, want LastPage=3, Percentage=20", stored)
	}
}

func TestPutProgress_UpdatesExistingProgress(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-progress-put-update")
	mustCreateTestProgressDirect(t, deps, book.ID, 1, 5)

	resp := putRequest(t, fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, book.ID), `{"lastPage": 10, "percentage": 90}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	stored, err := deps.progressRepo.GetByBookID(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("GetByBookID: unexpected error: %v", err)
	}
	if stored.LastPage != 10 || stored.Percentage != 90 {
		t.Errorf("stored progress = %+v, want LastPage=10, Percentage=90", stored)
	}
}

func TestPutProgress_BookNotFound_Returns404(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	resp := putRequest(t, httpSrv.URL+"/books/does-not-exist/progress", `{"lastPage": 1, "percentage": 10}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestPutProgress_InvalidPercentage_Returns400(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-progress-put-invalid")

	resp := putRequest(t, fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, book.ID), `{"lastPage": 1, "percentage": 150}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestHealth_ReturnsOK does not need Postgres or the fake extractor: the
// health check touches none of the Server's ports.
func TestHealth_ReturnsOK(t *testing.T) {
	server := httpserver.NewServer(nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
