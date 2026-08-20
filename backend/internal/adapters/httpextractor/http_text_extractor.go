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

	"pdf-reader/backend/internal/domain"
)

// HTTPTextExtractor implements ports.TextExtractor by calling the Python
// extractor service's POST /extract endpoint.
type HTTPTextExtractor struct {
	baseURL string
	client  *http.Client
}

// NewHTTPTextExtractor creates an HTTPTextExtractor targeting baseURL. If
// client is nil, http.DefaultClient is used.
func NewHTTPTextExtractor(baseURL string, client *http.Client) *HTTPTextExtractor {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTextExtractor{baseURL: baseURL, client: client}
}

type extractResponse struct {
	Pages []extractPage `json:"pages"`
}

type extractPage struct {
	PageNumber int            `json:"page_number"`
	Width      float64        `json:"width"`
	Height     float64        `json:"height"`
	Blocks     []extractBlock `json:"blocks"`
}

type extractBlock struct {
	Text string `json:"text"`
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("httpextractor: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("httpextractor: extractor service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed extractResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("httpextractor: parsing response JSON: %w", err)
	}

	pages := make([]*domain.Page, 0, len(parsed.Pages))
	for _, p := range parsed.Pages {
		texts := make([]string, 0, len(p.Blocks))
		for _, b := range p.Blocks {
			texts = append(texts, b.Text)
		}
		text := strings.Join(texts, "\n")

		page, err := domain.NewPage(bookID, p.PageNumber, text, p.Width, p.Height)
		if err != nil {
			return nil, fmt.Errorf("httpextractor: building domain page %d: %w", p.PageNumber, err)
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
