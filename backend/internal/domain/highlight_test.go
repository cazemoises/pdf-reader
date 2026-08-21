package domain

import (
	"errors"
	"testing"
)

func validCharRange() CharRange {
	return CharRange{Start: 10, End: 40}
}

func TestNewHighlight_ValidCreatesHighlight(t *testing.T) {
	r := validCharRange()
	h, err := NewHighlight("hl-1", "book-1", 4, r, "yellow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.ID != "hl-1" {
		t.Errorf("ID = %q, want %q", h.ID, "hl-1")
	}
	if h.BookID != "book-1" {
		t.Errorf("BookID = %q, want %q", h.BookID, "book-1")
	}
	if h.PageNumber != 4 {
		t.Errorf("PageNumber = %d, want %d", h.PageNumber, 4)
	}
	if h.Range != r {
		t.Errorf("Range = %+v, want %+v", h.Range, r)
	}
	if h.Color != "yellow" {
		t.Errorf("Color = %q, want %q", h.Color, "yellow")
	}
	if h.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewHighlight_RejectsEmptyID(t *testing.T) {
	_, err := NewHighlight("", "book-1", 4, validCharRange(), "yellow")
	if !errors.Is(err, ErrHighlightIDRequired) {
		t.Fatalf("err = %v, want ErrHighlightIDRequired", err)
	}
}

func TestNewHighlight_RejectsEmptyBookID(t *testing.T) {
	_, err := NewHighlight("hl-1", "", 4, validCharRange(), "yellow")
	if !errors.Is(err, ErrHighlightBookIDRequired) {
		t.Fatalf("err = %v, want ErrHighlightBookIDRequired", err)
	}
}

func TestNewHighlight_RejectsInvalidPageNumber(t *testing.T) {
	_, err := NewHighlight("hl-1", "book-1", 0, validCharRange(), "yellow")
	if !errors.Is(err, ErrHighlightPageNumberInvalid) {
		t.Fatalf("err = %v, want ErrHighlightPageNumberInvalid", err)
	}
}

func TestNewHighlight_RejectsInvalidCharRange(t *testing.T) {
	cases := []CharRange{
		{Start: 0, End: 0},
		{Start: 5, End: 5},
		{Start: -1, End: 10},
		{Start: 10, End: 5},
	}
	for _, r := range cases {
		if _, err := NewHighlight("hl-1", "book-1", 4, r, "yellow"); !errors.Is(err, ErrHighlightRangeInvalid) {
			t.Errorf("range %+v: err = %v, want ErrHighlightRangeInvalid", r, err)
		}
	}
}

func TestNewHighlight_RejectsEmptyColor(t *testing.T) {
	_, err := NewHighlight("hl-1", "book-1", 4, validCharRange(), "")
	if !errors.Is(err, ErrHighlightColorRequired) {
		t.Fatalf("err = %v, want ErrHighlightColorRequired", err)
	}
}
