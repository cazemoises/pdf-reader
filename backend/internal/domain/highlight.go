package domain

import (
	"errors"
	"time"
)

var (
	ErrHighlightIDRequired        = errors.New("domain: highlight id is required")
	ErrHighlightBookIDRequired    = errors.New("domain: highlight book id is required")
	ErrHighlightPageNumberInvalid = errors.New("domain: highlight page number must be positive")
	ErrHighlightBoxInvalid        = errors.New("domain: highlight bounding box is invalid")
	ErrHighlightColorRequired     = errors.New("domain: highlight color is required")
)

// BoundingBox describes the position and size of a highlighted region on a page.
type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// IsValid reports whether the box has a non-negative origin and a positive size.
func (bb BoundingBox) IsValid() bool {
	return bb.X >= 0 && bb.Y >= 0 && bb.Width > 0 && bb.Height > 0
}

// Highlight is a user-selected, colored region of text on a Book's page.
type Highlight struct {
	ID         string
	BookID     string
	PageNumber int
	Box        BoundingBox
	Color      string
	CreatedAt  time.Time
}

// NewHighlight creates a Highlight for the given book and page.
func NewHighlight(id, bookID string, pageNumber int, box BoundingBox, color string) (*Highlight, error) {
	if id == "" {
		return nil, ErrHighlightIDRequired
	}
	if bookID == "" {
		return nil, ErrHighlightBookIDRequired
	}
	if pageNumber <= 0 {
		return nil, ErrHighlightPageNumberInvalid
	}
	if !box.IsValid() {
		return nil, ErrHighlightBoxInvalid
	}
	if color == "" {
		return nil, ErrHighlightColorRequired
	}

	return &Highlight{
		ID:         id,
		BookID:     bookID,
		PageNumber: pageNumber,
		Box:        box,
		Color:      color,
		CreatedAt:  time.Now(),
	}, nil
}
