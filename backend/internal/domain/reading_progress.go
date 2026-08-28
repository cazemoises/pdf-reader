package domain

import (
	"errors"
	"time"
)

var (
	ErrReadingProgressBookIDRequired    = errors.New("domain: reading progress book id is required")
	ErrReadingProgressLastPageInvalid   = errors.New("domain: reading progress last page must not be negative")
	ErrReadingProgressPercentageInvalid = errors.New("domain: reading progress percentage must be between 0 and 100")
	ErrReadingProgressPageInvalid       = errors.New("domain: reading progress page number must be positive")
	ErrReadingProgressOffsetInvalid     = errors.New("domain: reading progress character offset must not be negative")
	ErrReadingProgressSourceInvalid     = errors.New("domain: reading progress source must be auto or manual")
)

type ReadingProgressSource string

const (
	ReadingProgressSourceAuto   ReadingProgressSource = "auto"
	ReadingProgressSourceManual ReadingProgressSource = "manual"
)

// ReadingProgress tracks how far a user has read into a Book.
type ReadingProgress struct {
	BookID          string                `json:"bookId"`
	LastPage        int                   `json:"lastPage"`
	Percentage      float64               `json:"percentage"`
	PageNumber      int                   `json:"pageNumber"`
	CharacterOffset int                   `json:"characterOffset"`
	Source          ReadingProgressSource `json:"source"`
	UpdatedAt       time.Time             `json:"updatedAt"`
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
		PageNumber: lastPage,
		Source:     ReadingProgressSourceAuto,
		UpdatedAt:  time.Now(),
	}, nil
}

func NewReadingProgressWithPosition(bookID string, lastPage int, percentage float64, pageNumber, characterOffset int, source ReadingProgressSource) (*ReadingProgress, error) {
	progress, err := NewReadingProgress(bookID, lastPage, percentage)
	if err != nil {
		return nil, err
	}
	if pageNumber <= 0 {
		return nil, ErrReadingProgressPageInvalid
	}
	if characterOffset < 0 {
		return nil, ErrReadingProgressOffsetInvalid
	}
	if source != ReadingProgressSourceAuto && source != ReadingProgressSourceManual {
		return nil, ErrReadingProgressSourceInvalid
	}
	progress.PageNumber = pageNumber
	progress.CharacterOffset = characterOffset
	progress.Source = source
	return progress, nil
}
