package filestorage_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pdf-reader/backend/internal/adapters/filestorage"
)

func TestSaveThenOpen_RoundTripsContent(t *testing.T) {
	baseDir := t.TempDir()
	storage := filestorage.NewFileSystemStorage(baseDir)

	want := "conteudo de teste"
	path, err := storage.Save(context.Background(), "book-1", strings.NewReader(want))
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	reader, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error opening: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}

	if string(got) != want {
		t.Errorf("content = %q, want %q", string(got), want)
	}
}

func TestOpen_NonExistentPathReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	storage := filestorage.NewFileSystemStorage(baseDir)

	_, err := storage.Open(context.Background(), filepath.Join(baseDir, "does-not-exist.pdf"))
	if err == nil {
		t.Fatal("expected error opening non-existent path, got nil")
	}
}

func TestSave_CreatesBaseDirIfMissing(t *testing.T) {
	parent := t.TempDir()
	baseDir := filepath.Join(parent, "nested", "storage")
	storage := filestorage.NewFileSystemStorage(baseDir)

	if _, err := os.Stat(baseDir); !os.IsNotExist(err) {
		t.Fatalf("baseDir unexpectedly exists before Save: %v", err)
	}

	path, err := storage.Save(context.Background(), "book-1", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected saved file to exist: %v", err)
	}
}
