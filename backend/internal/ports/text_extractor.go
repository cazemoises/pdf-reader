package ports

import (
	"context"
	"io"

	"pdf-reader/backend/internal/domain"
)

// TextExtractor delegates PDF text and layout extraction to an external
// service (e.g. the Python extraction service, reached over an internal
// HTTP call by the concrete adapter).
type TextExtractor interface {
	// Extract reads a PDF from source and returns its extracted Pages,
	// associated with bookID, including each page's text and dimensions.
	Extract(ctx context.Context, bookID string, source io.Reader) ([]*domain.Page, error)
}
