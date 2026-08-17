package ports

import (
	"context"
	"io"
)

// FileStorage saves and retrieves the raw PDF file backing a Book.
type FileStorage interface {
	// Save persists the content read from src as the source file for bookID,
	// returning the path/key it was stored under.
	Save(ctx context.Context, bookID string, src io.Reader) (path string, err error)

	// Open returns a reader for the file previously stored at path.
	// The caller is responsible for closing it.
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}
