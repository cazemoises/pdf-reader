package domain

import (
	"errors"
	"time"
)

var (
	ErrReadingProgressBookIDRequired    = errors.New("domain: reading progress book id is required")
	ErrReadingProgressLastPageInvalid   = errors.New("domain: reading progress last page must not be negative")
	ErrReadingProgressPercentageInvalid = errors.New("domain: reading progress percentage must be between 0 and 100")
)

// ReadingProgress tracks how far a user has read into a Book.
type ReadingProgress struct {
	BookID     string
	LastPage   int
	Percentage float64
	UpdatedAt  time.Time
}

// NewReadingProgress creates a ReadingProgress record for a book.
func NewReadingProgress(bookID string, lastPage int, percentage float64) (*ReadingProgress, error) {
	if bookID == "" {
		return nil, ErrReadingProgressBookIDRequired
	}
	if lastPage < 0 {
		return nil, ErrReadingProgressLastPageInvalid
	}
	if percentage < 0 || percentage > 100 {
		return nil, ErrReadingProgressPercentageInvalid
	}

	return &ReadingProgress{
		BookID:     bookID,
		LastPage:   lastPage,
		Percentage: percentage,
		UpdatedAt:  time.Now(),
	}, nil
}
