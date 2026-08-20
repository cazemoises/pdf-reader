package domain

import (
	"errors"
	"time"
)

var (
	ErrNoteIDRequired      = errors.New("domain: note id is required")
	ErrNoteBookIDRequired  = errors.New("domain: note book id is required")
	ErrNoteContentRequired = errors.New("domain: note content is required")
)

// Note is a piece of free-text annotation attached to a Book, and optionally
// scoped to one of its Highlights.
type Note struct {
	ID          string    `json:"id"`
	BookID      string    `json:"bookId"`
	HighlightID *string   `json:"highlightId,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
}

// NewNote creates a Note for a book. highlightID may be nil for a note that
// is not tied to a specific highlight.
func NewNote(id, bookID string, highlightID *string, content string) (*Note, error) {
	if id == "" {
		return nil, ErrNoteIDRequired
	}
	if bookID == "" {
		return nil, ErrNoteBookIDRequired
	}
	if content == "" {
		return nil, ErrNoteContentRequired
	}

	return &Note{
		ID:          id,
		BookID:      bookID,
		HighlightID: highlightID,
		Content:     content,
		CreatedAt:   time.Now(),
	}, nil
}
