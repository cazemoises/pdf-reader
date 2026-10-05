package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pdf-reader/backend/internal/adapters/httpserver"
	"pdf-reader/backend/internal/domain"
)

type blockingBody struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingBody) Read(p []byte) (int, error) { close(b.entered); <-b.release; return 0, io.EOF }
func (b *blockingBody) Close() error               { return nil }

type forbiddenBody struct{}

func (*forbiddenBody) Read(p []byte) (int, error) { panic("busy upload read the body") }
func (*forbiddenBody) Close() error               { return nil }

func TestConcurrentUploadRejectedBeforeParsing(t *testing.T) {
	server := httpserver.NewServer(nil, nil, nil, nil, nil, nil, nil)
	body := &blockingBody{make(chan struct{}), make(chan struct{})}
	first := httptest.NewRequest("POST", "/books", body)
	first.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	done := make(chan struct{})
	go func() { defer close(done); server.ServeHTTP(httptest.NewRecorder(), first) }()
	<-body.entered
	second := httptest.NewRequest("POST", "/books", &forbiddenBody{})
	second.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, second)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", response.Code)
	}
	if response.Header().Get("Retry-After") != "1" {
		t.Fatal("missing retry hint")
	}
	health := httptest.NewRecorder()
	server.ServeHTTP(health, httptest.NewRequest("GET", "/health", nil))
	if health.Code != 200 {
		t.Fatal("health blocked by upload")
	}
	close(body.release)
	<-done
	third := httptest.NewRecorder()
	server.ServeHTTP(third, httptest.NewRequest("POST", "/books", strings.NewReader("")))
	if third.Code != 400 {
		t.Fatalf("slot leaked: status=%d", third.Code)
	}
}

type singlePageRepo struct{}

func (*singlePageRepo) Create(context.Context, *domain.Page) error { return nil }
func (*singlePageRepo) ListByBookID(context.Context, string) ([]*domain.Page, error) {
	panic("one-page request loaded whole JSONB document")
}
func (*singlePageRepo) FindByBookIDAndNumber(ctx context.Context, bookID string, number int) (*domain.Page, error) {
	return domain.NewPage(bookID, number, "Legacy text", 600, 800)
}
func TestPageReadDoesNotLoadWholeDocument(t *testing.T) {
	server := httpserver.NewServer(nil, &singlePageRepo{}, nil, nil, nil, nil, nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("GET", "/books/book/pages/1", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Legacy text") {
		t.Fatal(response.Body.String())
	}
	if strings.Contains(response.Body.String(), "extraction") {
		t.Fatal("legacy page must omit null extraction")
	}
}

type failureBooks struct{ updated bool }

func (b *failureBooks) Create(context.Context, *domain.Book) error             { return nil }
func (b *failureBooks) FindByID(context.Context, string) (*domain.Book, error) { return nil, nil }
func (b *failureBooks) List(context.Context) ([]*domain.Book, error)           { return nil, nil }
func (b *failureBooks) Delete(context.Context, string) error                   { return nil }
func (b *failureBooks) Update(ctx context.Context, book *domain.Book) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	b.updated = book.Status == domain.BookStatusFailed
	return nil
}

type failureStorage struct{}

func (*failureStorage) Save(context.Context, string, io.Reader) (string, error) {
	return "stored.pdf", nil
}
func (*failureStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("pdf")), nil
}

type canceledExtractor struct{ cancel context.CancelFunc }

func (e *canceledExtractor) Extract(ctx context.Context, id string, source io.Reader) ([]*domain.Page, error) {
	e.cancel()
	return nil, context.Canceled
}
func TestCanceledExtractionPersistsFailedStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	books := &failureBooks{}
	server := httpserver.NewServer(books, nil, nil, nil, nil, &canceledExtractor{cancel}, &failureStorage{})
	body := "--test\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.pdf\"\r\nContent-Type: application/pdf\r\n\r\npdf\r\n--test--\r\n"
	request := httptest.NewRequest("POST", "/books", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != 502 || !books.updated {
		t.Fatalf("status=%d failed status persisted=%v", response.Code, books.updated)
	}
}
