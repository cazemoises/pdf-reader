package domain

import (
	"errors"
	"testing"
)

func TestNewPage_ValidCreatesPage(t *testing.T) {
	p, err := NewPage("book-1", 3, "extracted text", 612, 792)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.BookID != "book-1" {
		t.Errorf("BookID = %q, want %q", p.BookID, "book-1")
	}
	if p.Number != 3 {
		t.Errorf("Number = %d, want %d", p.Number, 3)
	}
	if p.Text != "extracted text" {
		t.Errorf("Text = %q, want %q", p.Text, "extracted text")
	}
	if p.Width != 612 || p.Height != 792 {
		t.Errorf("Width/Height = %v/%v, want 612/792", p.Width, p.Height)
	}
}

func TestNewPage_RejectsEmptyBookID(t *testing.T) {
	_, err := NewPage("", 1, "text", 612, 792)
	if !errors.Is(err, ErrPageBookIDRequired) {
		t.Fatalf("err = %v, want ErrPageBookIDRequired", err)
	}
}

func TestNewPage_RejectsZeroPageNumber(t *testing.T) {
	_, err := NewPage("book-1", 0, "text", 612, 792)
	if !errors.Is(err, ErrPageNumberInvalid) {
		t.Fatalf("err = %v, want ErrPageNumberInvalid", err)
	}
}

func TestNewPage_RejectsNegativePageNumber(t *testing.T) {
	_, err := NewPage("book-1", -1, "text", 612, 792)
	if !errors.Is(err, ErrPageNumberInvalid) {
		t.Fatalf("err = %v, want ErrPageNumberInvalid", err)
	}
}

func TestNewPage_RejectsNonPositiveDimensions(t *testing.T) {
	if _, err := NewPage("book-1", 1, "text", 0, 792); !errors.Is(err, ErrPageDimensionsInvalid) {
		t.Fatalf("width=0: err = %v, want ErrPageDimensionsInvalid", err)
	}
	if _, err := NewPage("book-1", 1, "text", 612, -1); !errors.Is(err, ErrPageDimensionsInvalid) {
		t.Fatalf("height=-1: err = %v, want ErrPageDimensionsInvalid", err)
	}
}
