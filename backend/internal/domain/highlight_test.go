package domain

import (
	"errors"
	"testing"
)

func validBoundingBox() BoundingBox {
	return BoundingBox{X: 10, Y: 20, Width: 100, Height: 15}
}

func TestNewHighlight_ValidCreatesHighlight(t *testing.T) {
	box := validBoundingBox()
	h, err := NewHighlight("hl-1", "book-1", 4, box, "yellow")
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
	if h.Box != box {
		t.Errorf("Box = %+v, want %+v", h.Box, box)
	}
	if h.Color != "yellow" {
		t.Errorf("Color = %q, want %q", h.Color, "yellow")
	}
	if h.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewHighlight_RejectsEmptyID(t *testing.T) {
	_, err := NewHighlight("", "book-1", 4, validBoundingBox(), "yellow")
	if !errors.Is(err, ErrHighlightIDRequired) {
		t.Fatalf("err = %v, want ErrHighlightIDRequired", err)
	}
}

func TestNewHighlight_RejectsEmptyBookID(t *testing.T) {
	_, err := NewHighlight("hl-1", "", 4, validBoundingBox(), "yellow")
	if !errors.Is(err, ErrHighlightBookIDRequired) {
		t.Fatalf("err = %v, want ErrHighlightBookIDRequired", err)
	}
}

func TestNewHighlight_RejectsInvalidPageNumber(t *testing.T) {
	_, err := NewHighlight("hl-1", "book-1", 0, validBoundingBox(), "yellow")
	if !errors.Is(err, ErrHighlightPageNumberInvalid) {
		t.Fatalf("err = %v, want ErrHighlightPageNumberInvalid", err)
	}
}

func TestNewHighlight_RejectsInvalidBoundingBox(t *testing.T) {
	cases := []BoundingBox{
		{X: 0, Y: 0, Width: 0, Height: 15},
		{X: 0, Y: 0, Width: 100, Height: 0},
		{X: -1, Y: 0, Width: 100, Height: 15},
		{X: 0, Y: -1, Width: 100, Height: 15},
	}
	for _, box := range cases {
		if _, err := NewHighlight("hl-1", "book-1", 4, box, "yellow"); !errors.Is(err, ErrHighlightBoxInvalid) {
			t.Errorf("box %+v: err = %v, want ErrHighlightBoxInvalid", box, err)
		}
	}
}

func TestNewHighlight_RejectsEmptyColor(t *testing.T) {
	_, err := NewHighlight("hl-1", "book-1", 4, validBoundingBox(), "")
	if !errors.Is(err, ErrHighlightColorRequired) {
		t.Fatalf("err = %v, want ErrHighlightColorRequired", err)
	}
}
