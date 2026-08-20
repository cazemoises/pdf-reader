package httpextractor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pdf-reader/backend/internal/adapters/httpextractor"
)

func TestExtract_HappyPathMultiplePagesAndBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Fatalf("expected file field: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"pages": [
				{
					"page_number": 1,
					"width": 612.0,
					"height": 792.0,
					"blocks": [
						{"text": "first block", "bbox": {"x": 10.0, "y": 20.0, "width": 100.0, "height": 15.0}},
						{"text": "second block", "bbox": {"x": 10.0, "y": 40.0, "width": 100.0, "height": 15.0}}
					]
				},
				{
					"page_number": 2,
					"width": 612.0,
					"height": 792.0,
					"blocks": [
						{"text": "page two text", "bbox": {"x": 10.0, "y": 20.0, "width": 100.0, "height": 15.0}}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	extractor := httpextractor.NewHTTPTextExtractor(server.URL, nil)

	pages, err := extractor.Extract(context.Background(), "book-1", strings.NewReader("fake pdf bytes"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}

	if pages[0].BookID != "book-1" {
		t.Errorf("pages[0].BookID = %q, want %q", pages[0].BookID, "book-1")
	}
	if pages[0].Number != 1 {
		t.Errorf("pages[0].Number = %d, want 1", pages[0].Number)
	}
	if pages[0].Width != 612.0 || pages[0].Height != 792.0 {
		t.Errorf("pages[0] dims = %v/%v, want 612/792", pages[0].Width, pages[0].Height)
	}
	wantText := "first block\nsecond block"
	if pages[0].Text != wantText {
		t.Errorf("pages[0].Text = %q, want %q", pages[0].Text, wantText)
	}

	if pages[1].Number != 2 {
		t.Errorf("pages[1].Number = %d, want 2", pages[1].Number)
	}
	if pages[1].Text != "page two text" {
		t.Errorf("pages[1].Text = %q, want %q", pages[1].Text, "page two text")
	}
}

func TestExtract_ReturnsErrorOnNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	extractor := httpextractor.NewHTTPTextExtractor(server.URL, nil)

	_, err := extractor.Extract(context.Background(), "book-1", strings.NewReader("fake pdf bytes"))
	if err == nil {
		t.Fatal("expected error for non-2xx status, got nil")
	}
}

func TestExtract_ReturnsErrorOnMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	extractor := httpextractor.NewHTTPTextExtractor(server.URL, nil)

	_, err := extractor.Extract(context.Background(), "book-1", strings.NewReader("fake pdf bytes"))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}
