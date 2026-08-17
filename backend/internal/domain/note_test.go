package domain

import (
	"errors"
	"testing"
)

func TestNewNote_ValidStandaloneNote(t *testing.T) {
	n, err := NewNote("note-1", "book-1", nil, "remember this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID != "note-1" {
		t.Errorf("ID = %q, want %q", n.ID, "note-1")
	}
	if n.BookID != "book-1" {
		t.Errorf("BookID = %q, want %q", n.BookID, "book-1")
	}
	if n.HighlightID != nil {
		t.Errorf("HighlightID = %v, want nil", n.HighlightID)
	}
	if n.Content != "remember this" {
		t.Errorf("Content = %q, want %q", n.Content, "remember this")
	}
	if n.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewNote_ValidNoteAttachedToHighlight(t *testing.T) {
	highlightID := "hl-1"
	n, err := NewNote("note-1", "book-1", &highlightID, "why this matters")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.HighlightID == nil || *n.HighlightID != "hl-1" {
		t.Errorf("HighlightID = %v, want pointer to %q", n.HighlightID, "hl-1")
	}
}

func TestNewNote_RejectsEmptyID(t *testing.T) {
	_, err := NewNote("", "book-1", nil, "content")
	if !errors.Is(err, ErrNoteIDRequired) {
		t.Fatalf("err = %v, want ErrNoteIDRequired", err)
	}
}

func TestNewNote_RejectsEmptyBookID(t *testing.T) {
	_, err := NewNote("note-1", "", nil, "content")
	if !errors.Is(err, ErrNoteBookIDRequired) {
		t.Fatalf("err = %v, want ErrNoteBookIDRequired", err)
	}
}

func TestNewNote_RejectsEmptyContent(t *testing.T) {
	_, err := NewNote("note-1", "book-1", nil, "")
	if !errors.Is(err, ErrNoteContentRequired) {
		t.Fatalf("err = %v, want ErrNoteContentRequired", err)
	}
}

func TestNewNote_RejectsEmptyNoteWithNoBookIDAndNoContent(t *testing.T) {
	_, err := NewNote("note-1", "", nil, "")
	if err == nil {
		t.Fatal("expected error for note with no BookID and no content, got nil")
	}
}
