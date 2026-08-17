package domain

import (
	"errors"
	"testing"
)

func TestNewReadingProgress_ValidCreatesProgress(t *testing.T) {
	rp, err := NewReadingProgress("book-1", 42, 55.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.BookID != "book-1" {
		t.Errorf("BookID = %q, want %q", rp.BookID, "book-1")
	}
	if rp.LastPage != 42 {
		t.Errorf("LastPage = %d, want %d", rp.LastPage, 42)
	}
	if rp.Percentage != 55.5 {
		t.Errorf("Percentage = %v, want %v", rp.Percentage, 55.5)
	}
	if rp.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestNewReadingProgress_AllowsZeroPageAndZeroPercentage(t *testing.T) {
	rp, err := NewReadingProgress("book-1", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.LastPage != 0 || rp.Percentage != 0 {
		t.Errorf("LastPage/Percentage = %d/%v, want 0/0", rp.LastPage, rp.Percentage)
	}
}

func TestNewReadingProgress_AllowsFullCompletion(t *testing.T) {
	rp, err := NewReadingProgress("book-1", 100, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.Percentage != 100 {
		t.Errorf("Percentage = %v, want 100", rp.Percentage)
	}
}

func TestNewReadingProgress_RejectsEmptyBookID(t *testing.T) {
	_, err := NewReadingProgress("", 1, 50)
	if !errors.Is(err, ErrReadingProgressBookIDRequired) {
		t.Fatalf("err = %v, want ErrReadingProgressBookIDRequired", err)
	}
}

func TestNewReadingProgress_RejectsNegativePage(t *testing.T) {
	_, err := NewReadingProgress("book-1", -1, 50)
	if !errors.Is(err, ErrReadingProgressLastPageInvalid) {
		t.Fatalf("err = %v, want ErrReadingProgressLastPageInvalid", err)
	}
}

func TestNewReadingProgress_RejectsPercentageOutOfRange(t *testing.T) {
	if _, err := NewReadingProgress("book-1", 1, -0.1); !errors.Is(err, ErrReadingProgressPercentageInvalid) {
		t.Fatalf("negative: err = %v, want ErrReadingProgressPercentageInvalid", err)
	}
	if _, err := NewReadingProgress("book-1", 1, 100.1); !errors.Is(err, ErrReadingProgressPercentageInvalid) {
		t.Fatalf("above 100: err = %v, want ErrReadingProgressPercentageInvalid", err)
	}
}
