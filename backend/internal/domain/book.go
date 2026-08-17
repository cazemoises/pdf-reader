package domain

import (
	"errors"
	"time"
)

// BookStatus represents the lifecycle state of a Book's PDF processing.
type BookStatus string

const (
	BookStatusProcessing BookStatus = "processing"
	BookStatusReady      BookStatus = "ready"
	BookStatusFailed     BookStatus = "failed"
)

var (
	ErrBookIDRequired     = errors.New("domain: book id is required")
	ErrBookTitleRequired  = errors.New("domain: book title is required")
	ErrBookSourceRequired = errors.New("domain: book source path is required")
	ErrInvalidBookStatus  = errors.New("domain: invalid book status")
)

// Book is a PDF document tracked by the reader.
type Book struct {
	ID         string
	Title      string
	SourcePath string
	Status     BookStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewBook creates a Book in the processing status, ready for extraction.
func NewBook(id, title, sourcePath string) (*Book, error) {
	if id == "" {
		return nil, ErrBookIDRequired
	}
	if title == "" {
		return nil, ErrBookTitleRequired
	}
	if sourcePath == "" {
		return nil, ErrBookSourceRequired
	}

	now := time.Now()
	return &Book{
		ID:         id,
		Title:      title,
		SourcePath: sourcePath,
		Status:     BookStatusProcessing,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// UpdateStatus transitions the book to a new status, rejecting unknown values.
func (b *Book) UpdateStatus(status BookStatus) error {
	switch status {
	case BookStatusProcessing, BookStatusReady, BookStatusFailed:
		b.Status = status
		b.UpdatedAt = time.Now()
		return nil
	default:
		return ErrInvalidBookStatus
	}
}
