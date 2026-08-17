package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewBook_ValidCreatesBookWithProcessingStatus(t *testing.T) {
	b, err := NewBook("book-1", "My Book", "/data/book-1.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.ID != "book-1" {
		t.Errorf("ID = %q, want %q", b.ID, "book-1")
	}
	if b.Title != "My Book" {
		t.Errorf("Title = %q, want %q", b.Title, "My Book")
	}
	if b.SourcePath != "/data/book-1.pdf" {
		t.Errorf("SourcePath = %q, want %q", b.SourcePath, "/data/book-1.pdf")
	}
	if b.Status != BookStatusProcessing {
		t.Errorf("Status = %q, want %q", b.Status, BookStatusProcessing)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if b.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestNewBook_RejectsEmptyID(t *testing.T) {
	_, err := NewBook("", "My Book", "/data/book-1.pdf")
	if !errors.Is(err, ErrBookIDRequired) {
		t.Fatalf("err = %v, want ErrBookIDRequired", err)
	}
}

func TestNewBook_RejectsEmptyTitle(t *testing.T) {
	_, err := NewBook("book-1", "", "/data/book-1.pdf")
	if !errors.Is(err, ErrBookTitleRequired) {
		t.Fatalf("err = %v, want ErrBookTitleRequired", err)
	}
}

func TestNewBook_RejectsEmptySourcePath(t *testing.T) {
	_, err := NewBook("book-1", "My Book", "")
	if !errors.Is(err, ErrBookSourceRequired) {
		t.Fatalf("err = %v, want ErrBookSourceRequired", err)
	}
}

func TestBook_UpdateStatus_AcceptsValidStatus(t *testing.T) {
	b, err := NewBook("book-1", "My Book", "/data/book-1.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prevUpdatedAt := b.UpdatedAt
	time.Sleep(time.Millisecond)

	if err := b.UpdateStatus(BookStatusReady); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Status != BookStatusReady {
		t.Errorf("Status = %q, want %q", b.Status, BookStatusReady)
	}
	if !b.UpdatedAt.After(prevUpdatedAt) {
		t.Error("UpdatedAt should be refreshed after status change")
	}
}

func TestBook_UpdateStatus_RejectsInvalidStatus(t *testing.T) {
	b, err := NewBook("book-1", "My Book", "/data/book-1.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := b.UpdateStatus(BookStatus("bogus")); !errors.Is(err, ErrInvalidBookStatus) {
		t.Fatalf("err = %v, want ErrInvalidBookStatus", err)
	}
}
