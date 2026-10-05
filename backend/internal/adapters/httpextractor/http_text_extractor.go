// Package httpextractor implements ports.TextExtractor by delegating PDF
// text extraction to the Python extractor service over HTTP.
package httpextractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"pdf-reader/backend/internal/domain"
)

// HTTPTextExtractor implements ports.TextExtractor by calling the Python
// extractor service's POST /extract endpoint.
type HTTPTextExtractor struct {
	baseURL string
	client  *http.Client
}

// NewHTTPTextExtractor creates an HTTPTextExtractor targeting baseURL. If
// client is nil, a client with a bounded processing timeout is used.
func NewHTTPTextExtractor(baseURL string, client *http.Client) *HTTPTextExtractor {
	if client == nil {
		client = &http.Client{Timeout: 100 * time.Second}
	}
	return &HTTPTextExtractor{baseURL: baseURL, client: client}
}

type extractResponse struct {
	SchemaVersion int               `json:"schema_version"`
	Pages         []json.RawMessage `json:"pages"`
}

type extractPage struct {
	PageNumber int            `json:"page_number"`
	Width      float64        `json:"width"`
	Height     float64        `json:"height"`
	Blocks     []extractBlock `json:"blocks"`
}

type extractBlock struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// Extract sends source's content to the extractor service and converts the
// returned pages into domain.Page values associated with bookID.
func (e *HTTPTextExtractor) Extract(ctx context.Context, bookID string, source io.Reader) ([]*domain.Page, error) {
	body, contentType, err := buildMultipartBody(source)
	if err != nil {
		return nil, fmt.Errorf("httpextractor: building request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/extract", body)
	if err != nil {
		return nil, fmt.Errorf("httpextractor: building request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpextractor: calling extractor service: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024+1))
	if err != nil {
		return nil, fmt.Errorf("httpextractor: reading response body: %w", err)
	}

	if len(respBody) > 32*1024*1024 {
		return nil, fmt.Errorf("httpextractor: response size limit exceeded")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("httpextractor: extractor service returned status %d", resp.StatusCode)
	}

	var parsed extractResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("httpextractor: parsing response JSON: %w", err)
	}

	pages := make([]*domain.Page, 0, len(parsed.Pages))
	for _, raw := range parsed.Pages {
		var p extractPage
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("httpextractor: invalid page: %w", err)
		}
		texts := make([]string, 0, len(p.Blocks))
		for _, b := range p.Blocks {
			if b.Type != "header" && b.Type != "footer" {
				text := b.Text
				if parsed.SchemaVersion >= 2 && b.Type == "table" {
					text = strings.ReplaceAll(text, "\n", "\n\n")
				}
				texts = append(texts, text)
			}
		}
		separator := "\n"
		if parsed.SchemaVersion >= 2 {
			separator = "\n\n"
		}
		text := strings.Join(texts, separator)

		page, err := domain.NewPage(bookID, p.PageNumber, text, p.Width, p.Height)
		if err != nil {
			return nil, fmt.Errorf("httpextractor: building domain page %d: %w", p.PageNumber, err)
		}
		if parsed.SchemaVersion >= 2 {
			page.Extraction = raw
		}
		pages = append(pages, page)
	}

	return pages, nil
}

func buildMultipartBody(source io.Reader) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "document.pdf")
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, source); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return body, writer.FormDataContentType(), nil
}
