// Package filestorage implements ports.FileStorage using the local
// filesystem.
package filestorage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileSystemStorage implements ports.FileStorage by storing files under a
// base directory on the local filesystem.
type FileSystemStorage struct {
	baseDir string
}

// NewFileSystemStorage creates a FileSystemStorage rooted at baseDir.
func NewFileSystemStorage(baseDir string) *FileSystemStorage {
	return &FileSystemStorage{baseDir: baseDir}
}

// Save persists the content read from src as the source file for bookID,
// creating the base directory if it does not yet exist, and returns the
// path it was stored under.
func (s *FileSystemStorage) Save(ctx context.Context, bookID string, src io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("filestorage: creating base directory: %w", err)
	}

	path := filepath.Join(s.baseDir, bookID+".pdf")

	dst, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("filestorage: creating file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("filestorage: writing file: %w", err)
	}

	return path, nil
}

// Open returns a reader for the file previously stored at path.
func (s *FileSystemStorage) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("filestorage: opening file: %w", err)
	}

	return f, nil
}
