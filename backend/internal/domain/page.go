package domain

import "errors"

var (
	ErrPageBookIDRequired    = errors.New("domain: page book id is required")
	ErrPageNumberInvalid     = errors.New("domain: page number must be positive")
	ErrPageDimensionsInvalid = errors.New("domain: page dimensions must be positive")
)

// Page is a single extracted page of a Book, including its text and the
// physical dimensions needed to later map highlight coordinates.
type Page struct {
	BookID string
	Number int
	Text   string
	Width  float64
	Height float64
}

// NewPage creates a Page for the given book at the given 1-based page number.
func NewPage(bookID string, number int, text string, width, height float64) (*Page, error) {
	if bookID == "" {
		return nil, ErrPageBookIDRequired
	}
	if number <= 0 {
		return nil, ErrPageNumberInvalid
	}
	if width <= 0 || height <= 0 {
		return nil, ErrPageDimensionsInvalid
	}

	return &Page{
		BookID: bookID,
		Number: number,
		Text:   text,
		Width:  width,
		Height: height,
	}, nil
}
