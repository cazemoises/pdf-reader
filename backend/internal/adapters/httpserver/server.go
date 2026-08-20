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
	extractor ports.TextExtractor,
	storage ports.FileStorage,
) *Server {
	s := &Server{
		bookRepo:      bookRepo,
		pageRepo:      pageRepo,
		highlightRepo: highlightRepo,
		extractor:     extractor,
		storage:       storage,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /books", s.handleCreateBook)
	mux.HandleFunc("GET /books/{id}", s.handleGetBook)
	mux.HandleFunc("GET /books/{id}/pages/{number}", s.handleGetPage)
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
