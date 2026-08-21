package domain

import (
	"errors"
	"time"
)

var (
	ErrHighlightIDRequired        = errors.New("domain: highlight id is required")
	ErrHighlightBookIDRequired    = errors.New("domain: highlight book id is required")
	ErrHighlightPageNumberInvalid = errors.New("domain: highlight page number must be positive")
	ErrHighlightRangeInvalid      = errors.New("domain: highlight character range is invalid")
	ErrHighlightColorRequired     = errors.New("domain: highlight color is required")
)

// CharRange identifies a highlighted span of text by its start and end
// character offsets within a page's plain text (as returned by
// GET /books/{id}/pages/{number}). Start is inclusive, End is exclusive,
// both 0-based - pageText[Start:End] is the highlighted substring.
type CharRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// IsValid reports whether the range has a non-negative start strictly
// before its end.
func (r CharRange) IsValid() bool {
	return r.Start >= 0 && r.End > r.Start
}

// Highlight is a user-selected, colored span of text on a Book's page,
// identified by its character offset range within that page's plain text.
type Highlight struct {
	ID         string    `json:"id"`
	BookID     string    `json:"bookId"`
	PageNumber int       `json:"pageNumber"`
	Range      CharRange `json:"range"`
	Color      string    `json:"color"`
	CreatedAt  time.Time `json:"createdAt"`
}

// NewHighlight creates a Highlight for the given book and page.
func NewHighlight(id, bookID string, pageNumber int, charRange CharRange, color string) (*Highlight, error) {
	if id == "" {
		return nil, ErrHighlightIDRequired
	}
	if bookID == "" {
		return nil, ErrHighlightBookIDRequired
	}
	if pageNumber <= 0 {
		return nil, ErrHighlightPageNumberInvalid
	}
	if !charRange.IsValid() {
		return nil, ErrHighlightRangeInvalid
	}
	if color == "" {
		return nil, ErrHighlightColorRequired
	}

	return &Highlight{
		ID:         id,
		BookID:     bookID,
		PageNumber: pageNumber,
		Range:      charRange,
		Color:      color,
		CreatedAt:  time.Now(),
	}, nil
}
