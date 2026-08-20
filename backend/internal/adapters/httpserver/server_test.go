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

	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql"} {
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

// boxJSON mirrors domain.BoundingBox's camelCase json tags.
type boxJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// highlightJSON mirrors domain.Highlight's camelCase json tags.
type highlightJSON struct {
	ID         string  `json:"id"`
	BookID     string  `json:"bookId"`
	PageNumber int     `json:"pageNumber"`
	Box        boxJSON `json:"box"`
	Color      string  `json:"color"`
}

func TestPostHighlights_CreatesHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-highlight-post")

	reqBody := `{"pageNumber": 1, "box": {"x": 10, "y": 20, "width": 100, "height": 30}, "color": "yellow"}`

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
	if got.Box != (boxJSON{X: 10, Y: 20, Width: 100, Height: 30}) {
		t.Errorf("Box = %+v, want {10 20 100 30}", got.Box)
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

	box := domain.BoundingBox{X: 1, Y: 2, Width: 3, Height: 4}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, box, "green")
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
