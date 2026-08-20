// Package httpserver implements the HTTP adapter that exposes the
// application's ports over a REST API. It depends only on ports.*, never on
// concrete adapters - those are wired up by cmd/server/main.go.
package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"pdf-reader/backend/internal/domain"
	"pdf-reader/backend/internal/ports"
)

// Server holds the ports needed to serve the reader's HTTP API and routes
// requests to their handlers.
type Server struct {
	bookRepo      ports.BookRepository
	pageRepo      ports.PageRepository
	highlightRepo ports.HighlightRepository
	noteRepo      ports.NoteRepository
	progressRepo  ports.ReadingProgressRepository
	extractor     ports.TextExtractor
	storage       ports.FileStorage
	mux           *http.ServeMux
}

// NewServer builds a Server wired to the given ports and registers its
// routes.
func NewServer(
	bookRepo ports.BookRepository,
	pageRepo ports.PageRepository,
	highlightRepo ports.HighlightRepository,
	noteRepo ports.NoteRepository,
	progressRepo ports.ReadingProgressRepository,
	extractor ports.TextExtractor,
	storage ports.FileStorage,
) *Server {
	s := &Server{
		bookRepo:      bookRepo,
		pageRepo:      pageRepo,
		highlightRepo: highlightRepo,
		noteRepo:      noteRepo,
		progressRepo:  progressRepo,
		extractor:     extractor,
		storage:       storage,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /books", s.handleCreateBook)
	mux.HandleFunc("GET /books/{id}", s.handleGetBook)
	mux.HandleFunc("GET /books/{id}/pages/{number}", s.handleGetPage)
	mux.HandleFunc("POST /books/{id}/highlights", s.handleCreateHighlight)
	mux.HandleFunc("GET /books/{id}/highlights", s.handleListHighlights)
	mux.HandleFunc("POST /books/{id}/notes", s.handleCreateNote)
	mux.HandleFunc("GET /books/{id}/notes", s.handleListNotes)
	mux.HandleFunc("GET /books/{id}/progress", s.handleGetProgress)
	mux.HandleFunc("PUT /books/{id}/progress", s.handleSaveProgress)
	mux.HandleFunc("GET /health", handleHealth)
	s.mux = mux

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleCreateBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	id := newID()

	path, err := s.storage.Save(ctx, id, file)
	if err != nil {
		http.Error(w, "saving file", http.StatusInternalServerError)
		return
	}

	book, err := domain.NewBook(id, title, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.bookRepo.Create(ctx, book); err != nil {
		http.Error(w, "creating book", http.StatusInternalServerError)
		return
	}

	pages, err := s.extractPages(ctx, book)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, book)
		return
	}

	for _, page := range pages {
		if err := s.pageRepo.Create(ctx, page); err != nil {
			http.Error(w, "storing page", http.StatusInternalServerError)
			return
		}
	}

	if err := book.UpdateStatus(domain.BookStatusReady); err != nil {
		http.Error(w, "updating book status", http.StatusInternalServerError)
		return
	}
	if err := s.bookRepo.Update(ctx, book); err != nil {
		http.Error(w, "updating book", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, book)
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	book, err := s.bookRepo.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, book)
}

func (s *Server) handleGetPage(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "invalid page number", http.StatusBadRequest)
		return
	}

	pages, err := s.pageRepo.ListByBookID(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	for _, page := range pages {
		if page.Number == number {
			writeJSON(w, http.StatusOK, page)
			return
		}
	}

	http.Error(w, "page not found", http.StatusNotFound)
}

type createHighlightRequest struct {
	PageNumber int                `json:"pageNumber"`
	Box        domain.BoundingBox `json:"box"`
	Color      string             `json:"color"`
}

func (s *Server) handleCreateHighlight(w http.ResponseWriter, r *http.Request) {
	var req createHighlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	highlight, err := domain.NewHighlight(newID(), r.PathValue("id"), req.PageNumber, req.Box, req.Color)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.highlightRepo.Create(r.Context(), highlight); err != nil {
		http.Error(w, "creating highlight", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, highlight)
}

func (s *Server) handleListHighlights(w http.ResponseWriter, r *http.Request) {
	highlights, err := s.highlightRepo.ListByBookID(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "listing highlights", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, highlights)
}

type createNoteRequest struct {
	HighlightID *string `json:"highlightId"`
	Content     string  `json:"content"`
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookID := r.PathValue("id")

	if _, err := s.bookRepo.FindByID(ctx, bookID); err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	var req createNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.HighlightID != nil {
		highlight, err := s.highlightRepo.FindByID(ctx, *req.HighlightID)
		if err != nil {
			http.Error(w, "highlight not found", http.StatusBadRequest)
			return
		}
		if highlight.BookID != bookID {
			http.Error(w, "highlight does not belong to this book", http.StatusBadRequest)
			return
		}
	}

	note, err := domain.NewNote(newID(), bookID, req.HighlightID, req.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.noteRepo.Create(ctx, note); err != nil {
		http.Error(w, "creating note", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, note)
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookID := r.PathValue("id")

	if _, err := s.bookRepo.FindByID(ctx, bookID); err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	notes, err := s.noteRepo.ListByBookID(ctx, bookID)
	if err != nil {
		http.Error(w, "listing notes", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookID := r.PathValue("id")

	if _, err := s.bookRepo.FindByID(ctx, bookID); err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	progress, err := s.progressRepo.GetByBookID(ctx, bookID)
	if err != nil {
		http.Error(w, "reading progress not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, progress)
}

type saveProgressRequest struct {
	LastPage   int     `json:"lastPage"`
	Percentage float64 `json:"percentage"`
}

func (s *Server) handleSaveProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookID := r.PathValue("id")

	if _, err := s.bookRepo.FindByID(ctx, bookID); err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	var req saveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	progress, err := domain.NewReadingProgress(bookID, req.LastPage, req.Percentage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.progressRepo.Save(ctx, progress); err != nil {
		http.Error(w, "saving reading progress", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, progress)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// extractPages opens the book's stored file and runs the extractor against
// it. On failure it marks the book as failed and persists that change
// before returning the error, so the caller can still report the book's id.
func (s *Server) extractPages(ctx context.Context, book *domain.Book) ([]*domain.Page, error) {
	reader, err := s.storage.Open(ctx, book.SourcePath)
	if err != nil {
		s.markFailed(ctx, book)
		return nil, err
	}
	defer reader.Close()

	pages, err := s.extractor.Extract(ctx, book.ID, reader)
	if err != nil {
		s.markFailed(ctx, book)
		return nil, err
	}

	return pages, nil
}

func (s *Server) markFailed(ctx context.Context, book *domain.Book) {
	_ = book.UpdateStatus(domain.BookStatusFailed)
	_ = s.bookRepo.Update(ctx, book)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
