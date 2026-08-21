// Package httpserver_test - see server_test.go's package doc comment for
// setup instructions (Postgres via DATABASE_URL, migrations, etc).
//
// This file contains a single end-to-end test that walks the full product
// flow described in the product vision's "ready" checklist: upload a book,
// have it processed by the extractor, read a page, create a highlight and a
// note on it, and save/retrieve reading progress.
package httpserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"pdf-reader/backend/internal/domain"
)

func TestEndToEnd_UploadProcessReadHighlightNoteProgress(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{
		"pages": [
			{"page_number": 1, "width": 612, "height": 792, "blocks": [{"text": "page one text"}]},
			{"page_number": 2, "width": 612, "height": 792, "blocks": [{"text": "page two text"}]}
		]
	}`)
	httpSrv, _ := newTestServer(t, db, extractorSrv.URL)

	// 1. POST /books with a fake PDF -> extraction runs -> book is "ready".
	body, contentType := multipartBody(t, "title", "End To End Book", "file", "book.pdf", "fake pdf bytes")

	createResp, err := http.Post(httpSrv.URL+"/books", contentType, body)
	if err != nil {
		t.Fatalf("POST /books: unexpected error: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /books status = %d, want %d", createResp.StatusCode, http.StatusCreated)
	}

	var createdBook bookJSON
	if err := json.NewDecoder(createResp.Body).Decode(&createdBook); err != nil {
		t.Fatalf("decoding POST /books response: %v", err)
	}
	if createdBook.ID == "" {
		t.Fatal("created book ID is empty, want a generated id")
	}
	if createdBook.Status != string(domain.BookStatusReady) {
		t.Errorf("created book Status = %q, want %q", createdBook.Status, domain.BookStatusReady)
	}
	bookID := createdBook.ID

	// 2. GET /books/{id} -> the book is recoverable and "ready".
	getBookResp, err := http.Get(httpSrv.URL + "/books/" + bookID)
	if err != nil {
		t.Fatalf("GET /books/{id}: unexpected error: %v", err)
	}
	defer getBookResp.Body.Close()

	if getBookResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /books/{id} status = %d, want %d", getBookResp.StatusCode, http.StatusOK)
	}

	var fetchedBook bookJSON
	if err := json.NewDecoder(getBookResp.Body).Decode(&fetchedBook); err != nil {
		t.Fatalf("decoding GET /books/{id} response: %v", err)
	}
	if fetchedBook.ID != bookID {
		t.Errorf("fetched book ID = %q, want %q", fetchedBook.ID, bookID)
	}
	if fetchedBook.Status != string(domain.BookStatusReady) {
		t.Errorf("fetched book Status = %q, want %q", fetchedBook.Status, domain.BookStatusReady)
	}

	// 3. GET /books/{id}/pages/{number} -> a specific page's text is recoverable.
	getPageResp, err := http.Get(fmt.Sprintf("%s/books/%s/pages/1", httpSrv.URL, bookID))
	if err != nil {
		t.Fatalf("GET /books/{id}/pages/{number}: unexpected error: %v", err)
	}
	defer getPageResp.Body.Close()

	if getPageResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /books/{id}/pages/1 status = %d, want %d", getPageResp.StatusCode, http.StatusOK)
	}

	var fetchedPage pageJSON
	if err := json.NewDecoder(getPageResp.Body).Decode(&fetchedPage); err != nil {
		t.Fatalf("decoding GET /books/{id}/pages/1 response: %v", err)
	}
	if fetchedPage.Number != 1 || fetchedPage.Text != "page one text" {
		t.Errorf("fetched page = %+v, want Number=1, Text=%q", fetchedPage, "page one text")
	}

	// 4. POST /books/{id}/highlights -> creates a highlight on page 1.
	highlightReqBody := `{"pageNumber": 1, "range": {"start": 10, "end": 40}, "color": "yellow"}`

	postHighlightResp, err := http.Post(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, bookID), "application/json", strings.NewReader(highlightReqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/highlights: unexpected error: %v", err)
	}
	defer postHighlightResp.Body.Close()

	if postHighlightResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /books/{id}/highlights status = %d, want %d", postHighlightResp.StatusCode, http.StatusCreated)
	}

	var createdHighlight highlightJSON
	if err := json.NewDecoder(postHighlightResp.Body).Decode(&createdHighlight); err != nil {
		t.Fatalf("decoding POST /books/{id}/highlights response: %v", err)
	}
	if createdHighlight.ID == "" {
		t.Fatal("created highlight ID is empty, want a generated id")
	}

	// 5. POST /books/{id}/notes -> creates a note associated with the highlight.
	noteReqBody := fmt.Sprintf(`{"content": "an important note", "highlightId": %q}`, createdHighlight.ID)

	postNoteResp, err := http.Post(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, bookID), "application/json", strings.NewReader(noteReqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/notes: unexpected error: %v", err)
	}
	defer postNoteResp.Body.Close()

	if postNoteResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /books/{id}/notes status = %d, want %d", postNoteResp.StatusCode, http.StatusCreated)
	}

	var createdNote noteJSON
	if err := json.NewDecoder(postNoteResp.Body).Decode(&createdNote); err != nil {
		t.Fatalf("decoding POST /books/{id}/notes response: %v", err)
	}
	if createdNote.ID == "" {
		t.Fatal("created note ID is empty, want a generated id")
	}
	if createdNote.HighlightID == nil || *createdNote.HighlightID != createdHighlight.ID {
		t.Errorf("created note HighlightID = %v, want %q", createdNote.HighlightID, createdHighlight.ID)
	}

	// 6. GET /books/{id}/highlights and GET /books/{id}/notes -> both list the created records.
	getHighlightsResp, err := http.Get(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, bookID))
	if err != nil {
		t.Fatalf("GET /books/{id}/highlights: unexpected error: %v", err)
	}
	defer getHighlightsResp.Body.Close()

	if getHighlightsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /books/{id}/highlights status = %d, want %d", getHighlightsResp.StatusCode, http.StatusOK)
	}

	var listedHighlights []highlightJSON
	if err := json.NewDecoder(getHighlightsResp.Body).Decode(&listedHighlights); err != nil {
		t.Fatalf("decoding GET /books/{id}/highlights response: %v", err)
	}
	if !containsHighlightID(listedHighlights, createdHighlight.ID) {
		t.Errorf("listed highlights %+v do not contain created highlight ID %q", listedHighlights, createdHighlight.ID)
	}

	getNotesResp, err := http.Get(fmt.Sprintf("%s/books/%s/notes", httpSrv.URL, bookID))
	if err != nil {
		t.Fatalf("GET /books/{id}/notes: unexpected error: %v", err)
	}
	defer getNotesResp.Body.Close()

	if getNotesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /books/{id}/notes status = %d, want %d", getNotesResp.StatusCode, http.StatusOK)
	}

	var listedNotes []noteJSON
	if err := json.NewDecoder(getNotesResp.Body).Decode(&listedNotes); err != nil {
		t.Fatalf("decoding GET /books/{id}/notes response: %v", err)
	}
	if !containsNoteID(listedNotes, createdNote.ID) {
		t.Errorf("listed notes %+v do not contain created note ID %q", listedNotes, createdNote.ID)
	}

	// 7. PUT /books/{id}/progress -> saves reading progress.
	putProgressResp := putRequest(t, fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, bookID), `{"lastPage": 2, "percentage": 50}`)
	defer putProgressResp.Body.Close()

	if putProgressResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /books/{id}/progress status = %d, want %d", putProgressResp.StatusCode, http.StatusOK)
	}

	// 8. GET /books/{id}/progress -> the saved progress is recoverable and reflects the PUT.
	getProgressResp, err := http.Get(fmt.Sprintf("%s/books/%s/progress", httpSrv.URL, bookID))
	if err != nil {
		t.Fatalf("GET /books/{id}/progress: unexpected error: %v", err)
	}
	defer getProgressResp.Body.Close()

	if getProgressResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /books/{id}/progress status = %d, want %d", getProgressResp.StatusCode, http.StatusOK)
	}

	var fetchedProgress progressJSON
	if err := json.NewDecoder(getProgressResp.Body).Decode(&fetchedProgress); err != nil {
		t.Fatalf("decoding GET /books/{id}/progress response: %v", err)
	}
	if fetchedProgress.BookID != bookID || fetchedProgress.LastPage != 2 || fetchedProgress.Percentage != 50 {
		t.Errorf("fetched progress = %+v, want BookID=%q, LastPage=2, Percentage=50", fetchedProgress, bookID)
	}
}

func containsHighlightID(highlights []highlightJSON, id string) bool {
	for _, h := range highlights {
		if h.ID == id {
			return true
		}
	}
	return false
}

func containsNoteID(notes []noteJSON, id string) bool {
	for _, n := range notes {
		if n.ID == id {
			return true
		}
	}
	return false
}
