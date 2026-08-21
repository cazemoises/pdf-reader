# Fluid Reflow Reader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the reading screen's canvas+pdf.js-textLayer rendering (fixed page geometry, hyphen-broken words at original PDF line breaks) with fluid, reflowing HTML text in a comfortable reading column, and re-anchor highlights to character offsets in the page's plain text instead of geometric bounding boxes.

**Architecture:** Backend: `domain.Highlight` moves from a `BoundingBox` (x/y/width/height) to a `CharRange` (start/end character offset within a page's plain text), with a new migration replacing the `box_*` columns with `start_offset`/`end_offset`. TDD required for every backend change (RED test first, then implementation). Frontend: `ReaderPage` drops pdf.js canvas rendering and `TextLayer`, fetches page text via the already-existing (but previously unused by the frontend) `GET /books/{id}/pages/{number}`, runs a simple de-hyphenation/reflow pass, renders paragraphs in a `max-width` reading column, and computes/display highlights via DOM-selection-to-character-offset math instead of bounding-rect math. pdf.js is kept for exactly one purpose — reading `numPages` from the PDF binary via `getDocument()` metadata, since the backend exposes no page-count endpoint and adding one is out of this task's authorized scope.

**Tech Stack:** Go 1.x (net/http, database/sql, lib/pq), Postgres, React 18 + TypeScript + Vite + Tailwind, pdfjs-dist (metadata only, no rendering).

**Spec:** This plan implements the task description given directly in conversation (no separate spec file) — see "Global Constraints" below for the load-bearing requirements extracted from it.

## Global Constraints

- Backend: hexagonal architecture — `internal/domain` stays free of external dependencies, `internal/ports` defines interfaces, `internal/adapters` implements them (Postgres, HTTP). Never let a handler talk to `database/sql` directly, etc.
- Backend: TDD is mandatory — write the failing test (RED), confirm it fails, then implement (GREEN), then refactor if needed. Never write backend implementation code before its failing test exists.
- Backend: atomic commits — never mix domain/adapter/frontend changes in one commit. This plan's task boundaries already reflect the required commit boundaries; do not combine tasks across a commit boundary.
- Frontend: no TDD in this phase (established convention) — validate by building and exploratory testing in a real browser (`docker compose up --build`), not automated tests.
- Do not touch `UploadPage.tsx` or `BookListPage.tsx`.
- Preserve: dark mode + `useTheme`/`ThemeToggle`, the Kindle-x-Notion OKLCH theme tokens and Work Sans/Source Serif 4 typography, reading-progress persistence (`lastPage`), and the notes/highlights bottom sheet.
- Discrete page navigation (Anterior/Próxima) stays; no continuous multi-page scroll (out of scope).
- Migration files must stay idempotent (`IF EXISTS`/`IF NOT EXISTS`) — they're re-applied on every backend process startup (`ApplyMigrations`, called from `cmd/server/main.go`).
- De-hyphenation heuristic is intentionally simple (no dictionary): a line ending in `-` immediately followed by a line starting with a lowercase letter gets joined, hyphen and line break removed. Document *why* inline only where the behavior isn't obvious from the code.
- Migrations are numbered sequentially; the next one is `0006_*.sql`.

---

## Design decision: character-range highlight storage

The existing `highlights` table has `box_x, box_y, box_width, box_height` (all `DOUBLE PRECISION`), used as a bounding box relative to the canvas-rendered page. Two options were available: (a) reinterpret those same four float columns as `(startOffset, endOffset, 0, 0)`, or (b) replace them with two dedicated integer columns.

**Decision: option (b).** Reusing `box_width`/`box_height` as always-zero placeholders would leave dead, misleading columns in the schema (a future reader has no way to know `width`/`height` are vestigial without reading this commit message), and `DOUBLE PRECISION` is the wrong type for a character offset (invites off-by-fractional bugs, wastes 4 bytes per row, needs a cast on every read). Two new `INTEGER NOT NULL` columns (`start_offset`, `end_offset`) replacing all four box columns is the same migration effort but leaves a schema that reads correctly on its own. This is spelled out again in the migration's own commit message per the task instructions.

`domain.BoundingBox` is renamed/replaced by `domain.CharRange{Start, End int}` (`Start` inclusive, `End` exclusive, both 0-based, matching normal Go slice semantics — `pageText[Start:End]` is the highlighted substring). `IsValid()` requires `Start >= 0 && End > Start` (mirrors the old box's "non-negative origin, positive size" rule).

---

## Task 1: Backend RED — domain test expresses char-range behavior

**Files:**
- Modify: `backend/internal/domain/highlight_test.go`

**Interfaces:**
- Consumes: nothing new yet (the type this test exercises, `domain.CharRange`, does not exist until Task 3 — that's the point of RED).
- Produces: the full expected test surface for `domain.NewHighlight` and `domain.CharRange`, which Task 3 must satisfy: `NewHighlight(id, bookID string, pageNumber int, r CharRange, color string) (*Highlight, error)`, `CharRange{Start, End int}`, `CharRange.IsValid() bool`, `ErrHighlightRangeInvalid`.

- [ ] **Step 1: Replace the file's contents to test `CharRange` instead of `BoundingBox`**

```go
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
```

- [ ] **Step 2: Run the domain tests and confirm RED**

Run: `go vet ./internal/domain/...` (from `backend/`)
Expected: FAIL — `undefined: CharRange` (and related undefined identifiers). This is the RED state: the test now expresses the desired behavior, and nothing in `domain` implements it yet.

---

## Task 2: Backend RED — adapter tests express char-range behavior

**Files:**
- Modify: `backend/internal/adapters/postgres/highlight_repository_test.go`
- Modify: `backend/internal/adapters/postgres/note_repository_test.go`

**Interfaces:**
- Consumes: same `domain.CharRange` / `domain.NewHighlight` surface as Task 1 (still unimplemented — still RED).
- Produces: nothing new for later tasks; these are leaf test files.

- [ ] **Step 1: Update `newTestHighlight` in `highlight_repository_test.go`**

In `backend/internal/adapters/postgres/highlight_repository_test.go`, replace:

```go
func newTestHighlight(t *testing.T, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	box := domain.BoundingBox{X: 10, Y: 20, Width: 100, Height: 30}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, box, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	return highlight
}
```

with:

```go
func newTestHighlight(t *testing.T, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	charRange := domain.CharRange{Start: 10, End: 40}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, charRange, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	return highlight
}
```

- [ ] **Step 2: Update the round-trip assertion in the same file**

Replace:

```go
	if got.ID != want.ID || got.BookID != want.BookID || got.PageNumber != want.PageNumber ||
		got.Box != want.Box || got.Color != want.Color {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
```

with:

```go
	if got.ID != want.ID || got.BookID != want.BookID || got.PageNumber != want.PageNumber ||
		got.Range != want.Range || got.Color != want.Color {
		t.Errorf("FindByID = %+v, want %+v", got, want)
	}
```

- [ ] **Step 3: Update `mustCreateTestHighlightForNotes` in `note_repository_test.go`**

Replace:

```go
func mustCreateTestHighlightForNotes(t *testing.T, ctx context.Context, db *sql.DB, id, bookID string) *domain.Highlight {
	t.Helper()

	box := domain.BoundingBox{X: 10, Y: 20, Width: 100, Height: 30}
	highlight, err := domain.NewHighlight(id, bookID, 1, box, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}

	highlightRepo := postgres.NewHighlightRepository(db)
	if err := highlightRepo.Create(ctx, highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}
```

with:

```go
func mustCreateTestHighlightForNotes(t *testing.T, ctx context.Context, db *sql.DB, id, bookID string) *domain.Highlight {
	t.Helper()

	charRange := domain.CharRange{Start: 10, End: 40}
	highlight, err := domain.NewHighlight(id, bookID, 1, charRange, "yellow")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}

	highlightRepo := postgres.NewHighlightRepository(db)
	if err := highlightRepo.Create(ctx, highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}
```

- [ ] **Step 4: Run and confirm RED**

Run: `go vet ./internal/adapters/postgres/...` (from `backend/`)
Expected: FAIL — `undefined: domain.CharRange` (both files). Same RED state as Task 1, now also covering the Postgres adapter's expected surface.

- [ ] **Step 5: Commit the RED state**

```bash
git add internal/domain/highlight_test.go internal/adapters/postgres/highlight_repository_test.go internal/adapters/postgres/note_repository_test.go
git commit -m "test(backend): RED - highlights identified by character offset range, not bounding box

domain.Highlight will stop describing a highlight's position as a
geometric bounding box (x/y/width/height on a rendered canvas) and start
describing it as a start/end character offset range within a page's
plain text, since the reader is dropping canvas rendering in favor of
fluid HTML text. These tests express that target API; domain.CharRange
does not exist yet, so this commit does not compile (go vet fails on
both internal/domain and internal/adapters/postgres) - that's the
expected RED state before the next commit's implementation."
```

---

## Task 3: Backend GREEN — migration, domain, and Postgres adapter

**Files:**
- Create: `backend/migrations/0006_highlight_char_offsets.sql`
- Modify: `backend/internal/domain/highlight.go`
- Modify: `backend/internal/adapters/postgres/highlight_repository.go`
- Modify: `backend/internal/adapters/postgres/highlight_repository_test.go` (migration list only)
- Modify: `backend/internal/adapters/postgres/note_repository_test.go` (migration list only)
- Modify: `backend/internal/adapters/postgres/reading_progress_repository_test.go` (migration list only)

**Interfaces:**
- Consumes: nothing external.
- Produces: `domain.CharRange{Start, End int}`, `domain.CharRange.IsValid() bool`, `domain.ErrHighlightRangeInvalid`, `domain.Highlight.Range CharRange` (json tag `"range"`), `domain.NewHighlight(id, bookID string, pageNumber int, r CharRange, color string) (*Highlight, error)`. `postgres.HighlightRepository` persists/reads `start_offset`/`end_offset` columns. Task 4 (HTTP handler) and Task 6/7 (frontend) depend on this exact shape.

- [ ] **Step 1: Create the migration**

```sql
-- Highlights are now anchored to a page's plain text (as served by
-- GET /books/{id}/pages/{number}) instead of a canvas-rendered PDF, so the
-- geometric bounding box columns no longer describe anything meaningful.
-- Replace them with a character offset range within that plain text.
-- (See docs/superpowers/plans/2026-08-21-fluid-reflow-reader.md for why
-- this replaces the box columns instead of reinterpreting them.)
ALTER TABLE highlights DROP COLUMN IF EXISTS box_x;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_y;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_width;
ALTER TABLE highlights DROP COLUMN IF EXISTS box_height;
ALTER TABLE highlights ADD COLUMN IF NOT EXISTS start_offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE highlights ADD COLUMN IF NOT EXISTS end_offset INTEGER NOT NULL DEFAULT 0;
ALTER TABLE highlights ALTER COLUMN start_offset DROP DEFAULT;
ALTER TABLE highlights ALTER COLUMN end_offset DROP DEFAULT;
```

- [ ] **Step 2: Rewrite `backend/internal/domain/highlight.go`**

```go
package domain

import (
	"errors"
	"time"
)

var (
	ErrHighlightIDRequired        = errors.New("domain: highlight id is required")
	ErrHighlightBookIDRequired    = errors.New("domain: highlight book id is required")
	ErrHighlightPageNumberInvalid = errors.New("domain: highlight page number must be positive")
	ErrHighlightRangeInvalid      = errors.New("domain: highlight character range is invalid")
	ErrHighlightColorRequired     = errors.New("domain: highlight color is required")
)

// CharRange identifies a highlighted span of text by its start and end
// character offsets within a page's plain text (as returned by
// GET /books/{id}/pages/{number}). Start is inclusive, End is exclusive,
// both 0-based - pageText[Start:End] is the highlighted substring.
type CharRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// IsValid reports whether the range has a non-negative start strictly
// before its end.
func (r CharRange) IsValid() bool {
	return r.Start >= 0 && r.End > r.Start
}

// Highlight is a user-selected, colored span of text on a Book's page,
// identified by its character offset range within that page's plain text.
type Highlight struct {
	ID         string    `json:"id"`
	BookID     string    `json:"bookId"`
	PageNumber int       `json:"pageNumber"`
	Range      CharRange `json:"range"`
	Color      string    `json:"color"`
	CreatedAt  time.Time `json:"createdAt"`
}

// NewHighlight creates a Highlight for the given book and page.
func NewHighlight(id, bookID string, pageNumber int, charRange CharRange, color string) (*Highlight, error) {
	if id == "" {
		return nil, ErrHighlightIDRequired
	}
	if bookID == "" {
		return nil, ErrHighlightBookIDRequired
	}
	if pageNumber <= 0 {
		return nil, ErrHighlightPageNumberInvalid
	}
	if !charRange.IsValid() {
		return nil, ErrHighlightRangeInvalid
	}
	if color == "" {
		return nil, ErrHighlightColorRequired
	}

	return &Highlight{
		ID:         id,
		BookID:     bookID,
		PageNumber: pageNumber,
		Range:      charRange,
		Color:      color,
		CreatedAt:  time.Now(),
	}, nil
}
```

- [ ] **Step 3: Run domain tests and confirm GREEN**

Run: `go test ./internal/domain/... -run TestNewHighlight -v` (from `backend/`)
Expected: PASS (all `TestNewHighlight_*` subtests).

- [ ] **Step 4: Rewrite `backend/internal/adapters/postgres/highlight_repository.go`**

```go
// Package postgres implements ports.HighlightRepository backed by a Postgres
// database via the standard library database/sql package.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"pdf-reader/backend/internal/domain"
)

// ErrHighlightNotFound is returned (wrapped) when a Highlight lookup,
// update, or delete targets an id that does not exist in the highlights
// table.
var ErrHighlightNotFound = errors.New("postgres: highlight not found")

// HighlightRepository implements ports.HighlightRepository against a
// highlights table in Postgres.
type HighlightRepository struct {
	db *sql.DB
}

// NewHighlightRepository creates a HighlightRepository using db as its
// connection pool. db is not owned by the repository; callers remain
// responsible for closing it.
func NewHighlightRepository(db *sql.DB) *HighlightRepository {
	return &HighlightRepository{db: db}
}

// Create stores a new Highlight.
func (r *HighlightRepository) Create(ctx context.Context, highlight *domain.Highlight) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO highlights (id, book_id, page_number, start_offset, end_offset, color, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		highlight.ID, highlight.BookID, highlight.PageNumber,
		highlight.Range.Start, highlight.Range.End,
		highlight.Color, highlight.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: creating highlight: %w", err)
	}
	return nil
}

// FindByID returns the Highlight with the given ID, or an error wrapping
// ErrHighlightNotFound if it does not exist.
func (r *HighlightRepository) FindByID(ctx context.Context, id string) (*domain.Highlight, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, book_id, page_number, start_offset, end_offset, color, created_at
		 FROM highlights WHERE id = $1`, id)

	highlight, err := scanHighlight(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: highlight %q: %w", id, ErrHighlightNotFound)
		}
		return nil, fmt.Errorf("postgres: finding highlight: %w", err)
	}
	return highlight, nil
}

// ListByBookID returns all Highlights belonging to the given Book.
func (r *HighlightRepository) ListByBookID(ctx context.Context, bookID string) ([]*domain.Highlight, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, book_id, page_number, start_offset, end_offset, color, created_at
		 FROM highlights WHERE book_id = $1`, bookID)
	if err != nil {
		return nil, fmt.Errorf("postgres: listing highlights: %w", err)
	}
	defer rows.Close()

	highlights := make([]*domain.Highlight, 0)
	for rows.Next() {
		highlight, err := scanHighlight(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scanning highlight: %w", err)
		}
		highlights = append(highlights, highlight)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: listing highlights: %w", err)
	}
	return highlights, nil
}

// Update persists changes made to an existing Highlight. It returns an
// error wrapping ErrHighlightNotFound if no Highlight with the given ID
// exists.
func (r *HighlightRepository) Update(ctx context.Context, highlight *domain.Highlight) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE highlights
		 SET book_id = $2, page_number = $3, start_offset = $4, end_offset = $5,
		     color = $6, created_at = $7
		 WHERE id = $1`,
		highlight.ID, highlight.BookID, highlight.PageNumber,
		highlight.Range.Start, highlight.Range.End,
		highlight.Color, highlight.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: updating highlight: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: updating highlight %q: %w", highlight.ID, ErrHighlightNotFound)
	}
	return nil
}

// Delete removes the Highlight with the given ID. It returns an error
// wrapping ErrHighlightNotFound if no Highlight with the given ID exists.
func (r *HighlightRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM highlights WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: deleting highlight: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgres: checking delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: deleting highlight %q: %w", id, ErrHighlightNotFound)
	}
	return nil
}

func scanHighlight(s rowScanner) (*domain.Highlight, error) {
	var highlight domain.Highlight
	if err := s.Scan(
		&highlight.ID, &highlight.BookID, &highlight.PageNumber,
		&highlight.Range.Start, &highlight.Range.End,
		&highlight.Color, &highlight.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &highlight, nil
}
```

- [ ] **Step 5: Add migration `0006_highlight_char_offsets.sql` to the three adapter test files' migration lists**

In `backend/internal/adapters/postgres/highlight_repository_test.go`, change:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql"} {
```

to:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0006_highlight_char_offsets.sql"} {
```

In `backend/internal/adapters/postgres/note_repository_test.go`, change:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql"} {
```

to:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0006_highlight_char_offsets.sql"} {
```

In `backend/internal/adapters/postgres/reading_progress_repository_test.go`, change:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql"} {
```

to:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql", "0006_highlight_char_offsets.sql"} {
```

- [ ] **Step 6: Run the Postgres adapter integration tests against a real database and confirm GREEN**

These tests skip unless `DATABASE_URL` is set. Start a throwaway Postgres and point the tests at it:

Run:
```bash
docker run --rm -d --name pdfreader-postgres-test \
  -e POSTGRES_USER=pdfreader -e POSTGRES_PASSWORD=pdfreader -e POSTGRES_DB=pdfreader \
  -p 5432:5432 postgres:16-alpine
# wait a few seconds for it to accept connections, then:
export DATABASE_URL="postgres://pdfreader:pdfreader@localhost:5432/pdfreader?sslmode=disable"
go test ./internal/adapters/postgres/... -v
```
Expected: PASS for all tests, including every `TestHighlightRepository_*` and `TestNoteRepository_*` and `TestApplyMigrations_CreatesAllExpectedTables` (the last one exercises the full migration set including 0006 twice in a row, verifying it's idempotent).

Leave the `pdfreader-postgres-test` container running — Task 4 reuses it. Keep `DATABASE_URL` exported in this shell for the same reason.

- [ ] **Step 7: Commit the GREEN state**

```bash
git add ../migrations/0006_highlight_char_offsets.sql internal/domain/highlight.go internal/adapters/postgres/highlight_repository.go internal/adapters/postgres/highlight_repository_test.go internal/adapters/postgres/note_repository_test.go internal/adapters/postgres/reading_progress_repository_test.go
git commit -m "feat(backend): store highlights as character offset ranges

Adds migration 0006 dropping the box_x/box_y/box_width/box_height
columns in favor of start_offset/end_offset (INTEGER), and updates
domain.Highlight and the Postgres adapter to match. Chose new dedicated
columns over reinterpreting the old float box columns as offsets - see
docs/superpowers/plans/2026-08-21-fluid-reflow-reader.md's design
decision section for why. Makes Task 1/2's RED tests pass.

Note: internal/adapters/httpserver does not compile after this commit
(it still references the removed domain.BoundingBox) - that's expected,
fixed by the next commit."
```

---

## Task 4: Backend — HTTP handler payload switches to character range

**Files:**
- Modify: `backend/internal/adapters/httpserver/server.go`
- Modify: `backend/internal/adapters/httpserver/server_test.go`
- Modify: `backend/internal/adapters/httpserver/integration_test.go`

**Interfaces:**
- Consumes: `domain.CharRange`, `domain.NewHighlight` (from Task 3).
- Produces: `POST /books/{id}/highlights` now accepts `{"pageNumber": N, "range": {"start": N, "end": N}, "color": "..."}` and both that endpoint and `GET /books/{id}/highlights` return `Highlight.range = {"start": N, "end": N}` instead of `Highlight.box`. Task 6/7 (frontend) depend on this exact JSON shape.

- [ ] **Step 1: Update `handleCreateHighlight`'s request struct and body in `server.go`**

Replace:

```go
type createHighlightRequest struct {
	PageNumber int                `json:"pageNumber"`
	Box        domain.BoundingBox `json:"box"`
	Color      string             `json:"color"`
}

func (s *Server) handleCreateHighlight(w http.ResponseWriter, r *http.Request) {
	var req createHighlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	highlight, err := domain.NewHighlight(newID(), r.PathValue("id"), req.PageNumber, req.Box, req.Color)
```

with:

```go
type createHighlightRequest struct {
	PageNumber int              `json:"pageNumber"`
	Range      domain.CharRange `json:"range"`
	Color      string           `json:"color"`
}

func (s *Server) handleCreateHighlight(w http.ResponseWriter, r *http.Request) {
	var req createHighlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	highlight, err := domain.NewHighlight(newID(), r.PathValue("id"), req.PageNumber, req.Range, req.Color)
```

- [ ] **Step 2: Update `server_test.go`'s JSON mirror types and requests**

Replace:

```go
// boxJSON mirrors domain.BoundingBox's camelCase json tags.
type boxJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// highlightJSON mirrors domain.Highlight's camelCase json tags.
type highlightJSON struct {
	ID         string  `json:"id"`
	BookID     string  `json:"bookId"`
	PageNumber int     `json:"pageNumber"`
	Box        boxJSON `json:"box"`
	Color      string  `json:"color"`
}

func TestPostHighlights_CreatesHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-highlight-post")

	reqBody := `{"pageNumber": 1, "box": {"x": 10, "y": 20, "width": 100, "height": 30}, "color": "yellow"}`

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/highlights: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got highlightJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Error("response ID is empty, want a generated id")
	}
	if got.BookID != book.ID || got.PageNumber != 1 || got.Color != "yellow" {
		t.Errorf("got = %+v, want BookID=%q, PageNumber=1, Color=yellow", got, book.ID)
	}
	if got.Box != (boxJSON{X: 10, Y: 20, Width: 100, Height: 30}) {
		t.Errorf("Box = %+v, want {10 20 100 30}", got.Box)
	}

	stored, err := deps.highlightRepo.FindByID(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if stored.BookID != book.ID {
		t.Errorf("stored highlight BookID = %q, want %q", stored.BookID, book.ID)
	}
}

func mustCreateTestHighlightDirect(t *testing.T, deps testDeps, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	box := domain.BoundingBox{X: 1, Y: 2, Width: 3, Height: 4}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, box, "green")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	if err := deps.highlightRepo.Create(context.Background(), highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}
```

with:

```go
// rangeJSON mirrors domain.CharRange's camelCase json tags.
type rangeJSON struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// highlightJSON mirrors domain.Highlight's camelCase json tags.
type highlightJSON struct {
	ID         string    `json:"id"`
	BookID     string    `json:"bookId"`
	PageNumber int       `json:"pageNumber"`
	Range      rangeJSON `json:"range"`
	Color      string    `json:"color"`
}

func TestPostHighlights_CreatesHighlight(t *testing.T) {
	db := openTestDBForServer(t)
	extractorSrv := fakeExtractorServer(t, http.StatusOK, `{"pages": []}`)
	httpSrv, deps := newTestServer(t, db, extractorSrv.URL)
	book := mustCreateTestBookDirect(t, deps, "book-highlight-post")

	reqBody := `{"pageNumber": 1, "range": {"start": 10, "end": 40}, "color": "yellow"}`

	resp, err := http.Post(fmt.Sprintf("%s/books/%s/highlights", httpSrv.URL, book.ID), "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /books/{id}/highlights: unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var got highlightJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID == "" {
		t.Error("response ID is empty, want a generated id")
	}
	if got.BookID != book.ID || got.PageNumber != 1 || got.Color != "yellow" {
		t.Errorf("got = %+v, want BookID=%q, PageNumber=1, Color=yellow", got, book.ID)
	}
	if got.Range != (rangeJSON{Start: 10, End: 40}) {
		t.Errorf("Range = %+v, want {10 40}", got.Range)
	}

	stored, err := deps.highlightRepo.FindByID(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if stored.BookID != book.ID {
		t.Errorf("stored highlight BookID = %q, want %q", stored.BookID, book.ID)
	}
}

func mustCreateTestHighlightDirect(t *testing.T, deps testDeps, id, bookID string, pageNumber int) *domain.Highlight {
	t.Helper()

	charRange := domain.CharRange{Start: 1, End: 4}
	highlight, err := domain.NewHighlight(id, bookID, pageNumber, charRange, "green")
	if err != nil {
		t.Fatalf("building test highlight: %v", err)
	}
	if err := deps.highlightRepo.Create(context.Background(), highlight); err != nil {
		t.Fatalf("creating test highlight: %v", err)
	}
	return highlight
}
```

- [ ] **Step 3: Add migration 0006 to `server_test.go`'s migration list**

Change:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql"} {
```

to:

```go
	for _, migration := range []string{"0001_create_books.sql", "0002_create_pages.sql", "0003_create_highlights.sql", "0004_create_notes.sql", "0005_create_reading_progress.sql", "0006_highlight_char_offsets.sql"} {
```

- [ ] **Step 4: Update `integration_test.go`'s highlight request body**

Change:

```go
	highlightReqBody := `{"pageNumber": 1, "box": {"x": 10, "y": 20, "width": 100, "height": 30}, "color": "yellow"}`
```

to:

```go
	highlightReqBody := `{"pageNumber": 1, "range": {"start": 10, "end": 40}, "color": "yellow"}`
```

- [ ] **Step 5: Run the full backend test suite and confirm GREEN**

Reuse the `pdfreader-postgres-test` container and `DATABASE_URL` from Task 3, Step 6 (start it again the same way if it was stopped). Run:

```bash
go build ./...
go test ./... -v
```
Expected: `go build ./...` succeeds (httpserver now compiles again). `go test ./...` passes everywhere, including `TestPostHighlights_CreatesHighlight`, `TestGetHighlights_ListsHighlightsForBook`, `TestEndToEnd_UploadProcessReadHighlightNoteProgress`, and all Task 1-3 tests.

Then tear down the throwaway database:
```bash
docker stop pdfreader-postgres-test
unset DATABASE_URL
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/httpserver/server.go internal/adapters/httpserver/server_test.go internal/adapters/httpserver/integration_test.go
git commit -m "feat(backend): highlights HTTP API uses character range payload

POST /books/{id}/highlights now takes {\"range\": {\"start\", \"end\"}}
instead of {\"box\": {\"x\",\"y\",\"width\",\"height\"}}; GET responses
match. Completes the char-offset highlight migration started in the
previous two commits - full backend test suite is green again."
```

---

## Task 5: Frontend — API client and types for page text and char-range highlights

**Files:**
- Modify: `frontend/src/api/types.ts`
- Modify: `frontend/src/api/client.ts`

**Interfaces:**
- Consumes: the backend JSON shapes from Task 4 (`Highlight.range = {start, end}`) and the pre-existing `GET /books/{id}/pages/{number}` response shape (`{bookId, number, text, width, height}`, matching `domain.Page`'s json tags).
- Produces: `CharRange`, `Highlight` (with `.range`), `Page`, `getPage(bookId, pageNumber): Promise<Page>`, `createHighlight(bookId, pageNumber, range, color): Promise<Highlight>`. Tasks 6 and 7 (ReaderPage) depend on these exact names/signatures.

- [ ] **Step 1: Update `frontend/src/api/types.ts`**

Replace the `BoundingBox`/`Highlight` section:

```ts
export interface BoundingBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Highlight {
  id: string;
  bookId: string;
  pageNumber: number;
  box: BoundingBox;
  color: string;
  createdAt: string;
}
```

with:

```ts
export interface CharRange {
  start: number;
  end: number;
}

export interface Highlight {
  id: string;
  bookId: string;
  pageNumber: number;
  range: CharRange;
  color: string;
  createdAt: string;
}

export interface Page {
  bookId: string;
  number: number;
  text: string;
  width: number;
  height: number;
}
```

- [ ] **Step 2: Update `frontend/src/api/client.ts`'s import and add `getPage`**

Change the top import:

```ts
import type { Book, BoundingBox, Highlight, Note, ReadingProgress } from "./types";
```

to:

```ts
import type { Book, CharRange, Highlight, Note, Page, ReadingProgress } from "./types";
```

Add, right after `bookFileUrl`:

```ts
export async function getPage(bookId: string, pageNumber: number): Promise<Page> {
  const res = await fetch(`/books/${bookId}/pages/${pageNumber}`);
  return parseJSONOrThrow<Page>(res);
}
```

- [ ] **Step 3: Update `createHighlight`'s signature and body**

Replace:

```ts
export async function createHighlight(
  bookId: string,
  pageNumber: number,
  box: BoundingBox,
  color: string,
): Promise<Highlight> {
  const res = await fetch(`/books/${bookId}/highlights`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pageNumber, box, color }),
  });
  return parseJSONOrThrow<Highlight>(res);
}
```

with:

```ts
export async function createHighlight(
  bookId: string,
  pageNumber: number,
  range: CharRange,
  color: string,
): Promise<Highlight> {
  const res = await fetch(`/books/${bookId}/highlights`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pageNumber, range, color }),
  });
  return parseJSONOrThrow<Highlight>(res);
}
```

- [ ] **Step 4: Verify the frontend still typechecks**

Run (from `frontend/`): `npx tsc -b --noEmit`
Expected: FAILS at this point — `ReaderPage.tsx` still imports `BoundingBox` and uses `.box`. That's expected; Tasks 6-7 fix it. (If you want a clean signal for just this task's files, `npx tsc --noEmit src/api/types.ts src/api/client.ts` has no errors of its own, but `tsc` type-checks the whole program, so the project-wide command above is expected to fail until Task 7 lands. Do not attempt to fix ReaderPage.tsx from this task.)

- [ ] **Step 5: Commit**

```bash
git add src/api/types.ts src/api/client.ts
git commit -m "feat(frontend): API client for page text and char-range highlights

Adds getPage() (GET /books/{id}/pages/{number}, previously unused by
the frontend) and switches Highlight/createHighlight from a geometric
BoundingBox to a CharRange {start, end}, matching the backend's new
highlight payload shape. ReaderPage.tsx is updated in the next two
commits; the project does not typecheck cleanly until then."
```

---

## Task 6: Frontend — page text reflow/de-hyphenation and highlight offset math (pure helpers)

**Files:**
- Create: `frontend/src/lib/pageText.ts`
- Create: `frontend/src/lib/highlightLayout.ts`

**Interfaces:**
- Consumes: `Highlight`, `CharRange` from `../api/types`.
- Produces: `reflowPageText(raw: string): string[]`, `paragraphOffsets(paragraphs: string[]): number[]`, `TextSegment`, `segmentParagraph(paragraphText: string, paragraphStart: number, highlights: Highlight[]): TextSegment[]`, `selectionToRange(container: HTMLElement, selection: Selection): CharRange | null`, `highlightBackground(hex: string): string`. Task 7 (ReaderPage) consumes all of these directly.

No automated tests for this task (frontend has no TDD in this phase per project convention) - Task 7's manual browser validation exercises this code.

- [ ] **Step 1: Create `frontend/src/lib/pageText.ts`**

```ts
// Extractor blocks are frequently one PDF line each (see
// backend/internal/adapters/httpextractor: blocks are joined with "\n"),
// so a page's raw text is full of the source PDF's hard line breaks -
// including ones that split a single word across two lines with a
// trailing hyphen. reflowPageText joins any line ending in "-" with the
// next line when that next line starts with a lowercase letter (the
// common case for a word broken mid-syllable) and otherwise treats a line
// break as a mid-paragraph wrap, collapsing it into a single space so the
// browser reflows the text instead of keeping the PDF's fixed line
// geometry. A blank line in the source is kept as a paragraph break. This
// is a simple heuristic, not a dictionary-backed one: a legitimate
// end-of-line hyphen (e.g. a compound word) gets merged too, which is an
// accepted trade-off for a first version.
export function reflowPageText(raw: string): string[] {
  const rawLines = raw.split("\n");

  const mergedLines: string[] = [];
  for (const line of rawLines) {
    const previous = mergedLines[mergedLines.length - 1];
    if (previous !== undefined && previous.endsWith("-") && /^[a-z]/.test(line)) {
      mergedLines[mergedLines.length - 1] = previous.slice(0, -1) + line;
    } else {
      mergedLines.push(line);
    }
  }

  const paragraphs: string[] = [];
  let current: string[] = [];
  for (const line of mergedLines) {
    if (line.trim() === "") {
      if (current.length > 0) {
        paragraphs.push(current.join(" ").trim());
        current = [];
      }
      continue;
    }
    current.push(line.trim());
  }
  if (current.length > 0) {
    paragraphs.push(current.join(" ").trim());
  }

  return paragraphs.length > 0 ? paragraphs : [""];
}

// The canonical offset space highlights are anchored to: paragraphs
// joined with a 2-character separator ("\n\n"), even though that
// separator is never rendered as literal text - see highlightLayout.ts,
// which walks the DOM using this same +2-per-paragraph-boundary rule so
// offsets computed from a live selection match offsets computed here.
export function paragraphOffsets(paragraphs: string[]): number[] {
  const offsets: number[] = [];
  let running = 0;
  for (const paragraph of paragraphs) {
    offsets.push(running);
    running += paragraph.length + 2;
  }
  return offsets;
}
```

- [ ] **Step 2: Create `frontend/src/lib/highlightLayout.ts`**

```ts
import type { CharRange, Highlight } from "../api/types";

export interface TextSegment {
  text: string;
  highlight: Highlight | null;
}

// Splits a single paragraph's text into segments so each one can be
// rendered as plain text or as a <mark> for a highlight, given absolute
// character offsets (paragraphStart is this paragraph's offset in the
// same canonical offset space as paragraphOffsets() in pageText.ts).
export function segmentParagraph(
  paragraphText: string,
  paragraphStart: number,
  highlights: Highlight[],
): TextSegment[] {
  const paragraphEnd = paragraphStart + paragraphText.length;
  const overlapping = highlights.filter(
    (h) => h.range.start < paragraphEnd && h.range.end > paragraphStart,
  );

  const boundaries = new Set<number>([0, paragraphText.length]);
  for (const h of overlapping) {
    boundaries.add(Math.max(0, h.range.start - paragraphStart));
    boundaries.add(Math.min(paragraphText.length, h.range.end - paragraphStart));
  }
  const points = Array.from(boundaries).sort((a, b) => a - b);

  const segments: TextSegment[] = [];
  for (let i = 0; i < points.length - 1; i++) {
    const from = points[i];
    const to = points[i + 1];
    if (from === to) {
      continue;
    }
    const absoluteFrom = paragraphStart + from;
    const absoluteTo = paragraphStart + to;
    const covering =
      overlapping.find((h) => h.range.start <= absoluteFrom && h.range.end >= absoluteTo) ?? null;
    segments.push({ text: paragraphText.slice(from, to), highlight: covering });
  }
  return segments;
}

// Converts the current window selection (assumed to be inside `container`,
// whose direct children are one element per paragraph, in the same order
// as the paragraphs array used to render them) into a CharRange in the
// same canonical offset space as paragraphOffsets(). Only handles
// selections that start/end inside a text node, which covers ordinary
// click-and-drag text selection in every real browser; selections whose
// boundary lands exactly on an element (rare, mostly synthetic) are not
// supported and return null.
export function selectionToRange(container: HTMLElement, selection: Selection): CharRange | null {
  if (selection.rangeCount === 0) {
    return null;
  }
  const domRange = selection.getRangeAt(0);
  if (domRange.collapsed) {
    return null;
  }

  const start = domPositionToOffset(container, domRange.startContainer, domRange.startOffset);
  const end = domPositionToOffset(container, domRange.endContainer, domRange.endOffset);
  if (start === null || end === null || end <= start) {
    return null;
  }
  return { start, end };
}

function domPositionToOffset(container: HTMLElement, targetNode: Node, targetOffset: number): number | null {
  let offset = 0;

  const paragraphs = Array.from(container.children);
  for (let index = 0; index < paragraphs.length; index++) {
    const paragraph = paragraphs[index];
    if (index > 0) {
      offset += 2; // the "\n\n" paragraph separator, not present as DOM text
    }

    if (paragraph.contains(targetNode)) {
      const walker = document.createTreeWalker(paragraph, NodeFilter.SHOW_TEXT);
      let node = walker.nextNode();
      while (node) {
        if (node === targetNode) {
          return offset + targetOffset;
        }
        offset += node.textContent?.length ?? 0;
        node = walker.nextNode();
      }
      return offset;
    }

    offset += paragraph.textContent?.length ?? 0;
  }

  return null;
}

// HIGHLIGHT_COLORS in ReaderPage.tsx are solid hex swatches meant for the
// small color-picker dots; used directly as a <mark> background they'd be
// too saturated behind body text, so this converts to a translucent rgba
// for the actual highlight fill.
export function highlightBackground(hex: string): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, 0.35)`;
}
```

- [ ] **Step 3: Typecheck the new files in isolation**

Run (from `frontend/`): `npx tsc --noEmit --strict src/lib/pageText.ts src/lib/highlightLayout.ts`
Expected: no errors reported for these two files (the command may still print errors from other unrelated files it pulls in via the project's `tsconfig`; only confirm `pageText.ts`/`highlightLayout.ts` are clean).

- [ ] **Step 4: Commit**

```bash
git add src/lib/pageText.ts src/lib/highlightLayout.ts
git commit -m "feat(frontend): page text reflow/de-hyphenation and highlight offset math

Pure helpers ReaderPage.tsx (next commit) will use to turn a page's raw
extracted text into reflowable paragraphs, and to convert between DOM
selections and the character-offset ranges the backend now stores
highlights as."
```

---

## Task 7: Frontend — ReaderPage renders fluid text and highlights by offset

**Files:**
- Modify: `frontend/src/pages/ReaderPage.tsx`
- Modify: `frontend/src/pages/ReaderPage.css`

**Interfaces:**
- Consumes: `getPage`, `createHighlight` (Task 5), `reflowPageText`, `paragraphOffsets` (Task 6, `pageText.ts`), `segmentParagraph`, `selectionToRange`, `highlightBackground` (Task 6, `highlightLayout.ts`).
- Produces: the reading screen itself - nothing else depends on this.

- [ ] **Step 1: Replace `frontend/src/pages/ReaderPage.tsx` entirely**

```tsx
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { GlobalWorkerOptions, getDocument } from "pdfjs-dist";
import pdfWorkerSrc from "pdfjs-dist/build/pdf.worker.mjs?url";
import {
  bookFileUrl,
  createHighlight,
  createNote,
  getBook,
  getPage,
  getProgress,
  listHighlights,
  listNotes,
  saveProgress,
} from "../api/client";
import type { Book, CharRange, Highlight, Note } from "../api/types";
import { paragraphOffsets, reflowPageText } from "../lib/pageText";
import { highlightBackground, segmentParagraph, selectionToRange } from "../lib/highlightLayout";
import ThemeToggle from "../components/ThemeToggle";
import "./ReaderPage.css";

GlobalWorkerOptions.workerSrc = pdfWorkerSrc;

const HIGHLIGHT_COLORS = ["#d9a441", "#6fa87c", "#d98a6f", "#7fa3c2"];

interface PendingSelection {
  pageNumber: number;
  range: CharRange;
  anchorX: number;
  anchorY: number;
  containerWidth: number;
}

interface PageText {
  paragraphs: string[];
  offsets: number[];
}

function ReaderPage() {
  const { id } = useParams<{ id: string }>();
  const textContainerRef = useRef<HTMLDivElement | null>(null);

  const [book, setBook] = useState<Book | null>(null);
  const [numPages, setNumPages] = useState<number | null>(null);
  const [pageNumber, setPageNumber] = useState(1);
  const [pageText, setPageText] = useState<PageText | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [progressReady, setProgressReady] = useState(false);
  const skipNextSaveRef = useRef(false);

  const [highlights, setHighlights] = useState<Highlight[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [chromeVisible, setChromeVisible] = useState(false);
  const [pendingSelection, setPendingSelection] = useState<PendingSelection | null>(null);
  const [selectedColor, setSelectedColor] = useState(HIGHLIGHT_COLORS[0]);
  const [noteDraft, setNoteDraft] = useState("");

  // pdf.js is kept for exactly one purpose: reading numPages from the PDF
  // binary's metadata. The backend has no page-count endpoint, and adding
  // one is out of scope here, so this loads the document only to read
  // that field and immediately destroys it - no canvas, no rendering.
  useEffect(() => {
    if (!id) {
      return;
    }

    let cancelled = false;

    getBook(id)
      .then((result) => {
        if (!cancelled) {
          setBook(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load book");
        }
      });

    const loadingTask = getDocument({ url: bookFileUrl(id) });
    loadingTask.promise
      .then((doc) => {
        if (!cancelled) {
          setNumPages(doc.numPages);
        }
        void doc.destroy();
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load PDF metadata");
        }
      });

    return () => {
      cancelled = true;
      void loadingTask.destroy();
    };
  }, [id]);

  useEffect(() => {
    if (numPages === null || !id) {
      return;
    }

    let cancelled = false;
    setProgressReady(false);

    getProgress(id)
      .then((progress) => {
        if (cancelled || !progress) {
          return;
        }
        if (progress.lastPage >= 1 && progress.lastPage <= numPages) {
          skipNextSaveRef.current = true;
          setPageNumber(progress.lastPage);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load reading progress");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setProgressReady(true);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [numPages, id]);

  useEffect(() => {
    if (!id) {
      return;
    }

    let cancelled = false;
    setPageText(null);

    getPage(id, pageNumber)
      .then((page) => {
        if (cancelled) {
          return;
        }
        const paragraphs = reflowPageText(page.text);
        setPageText({ paragraphs, offsets: paragraphOffsets(paragraphs) });
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load page text");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [id, pageNumber]);

  useEffect(() => {
    if (!id) {
      return;
    }

    let cancelled = false;

    listHighlights(id)
      .then((result) => {
        if (!cancelled) {
          setHighlights(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load highlights");
        }
      });

    listNotes(id)
      .then((result) => {
        if (!cancelled) {
          setNotes(result);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load notes");
        }
      });

    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    if (numPages === null || !id || !progressReady) {
      return;
    }

    if (skipNextSaveRef.current) {
      skipNextSaveRef.current = false;
      return;
    }

    const percentage = (pageNumber / numPages) * 100;
    saveProgress(id, pageNumber, percentage).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : "failed to save reading progress");
    });
  }, [numPages, id, pageNumber, progressReady]);

  function handleContainerMouseUp() {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0 || !selection.toString().trim()) {
      // No text was dragged into a selection: treat this as a tap that
      // shows/hides the reading chrome instead of starting a highlight.
      setChromeVisible((visible) => !visible);
      return;
    }

    const container = textContainerRef.current;
    if (!container) {
      return;
    }

    const range = selectionToRange(container, selection);
    if (!range) {
      return;
    }

    const containerRect = container.getBoundingClientRect();
    const selectionRect = selection.getRangeAt(0).getBoundingClientRect();
    if (containerRect.width === 0) {
      return;
    }

    setPendingSelection({
      pageNumber,
      range,
      anchorX: selectionRect.right - containerRect.left,
      anchorY: selectionRect.bottom - containerRect.top,
      containerWidth: containerRect.width,
    });
    setSelectedColor(HIGHLIGHT_COLORS[0]);
    setNoteDraft("");
  }

  function handleCancelHighlight() {
    setPendingSelection(null);
    setNoteDraft("");
  }

  async function handleConfirmHighlight() {
    if (!id || !pendingSelection) {
      return;
    }

    try {
      const highlight = await createHighlight(
        id,
        pendingSelection.pageNumber,
        pendingSelection.range,
        selectedColor,
      );
      setHighlights((prev) => [...prev, highlight]);

      const content = noteDraft.trim();
      if (content) {
        const note = await createNote(id, highlight.id, content);
        setNotes((prev) => [...prev, note]);
      }

      window.getSelection()?.removeAllRanges();
      setPendingSelection(null);
      setNoteDraft("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to create highlight");
    }
  }

  if (!id) {
    return <p className="p-6 text-danger">Missing book id.</p>;
  }

  const readingProgressPct = numPages ? (pageNumber / numPages) * 100 : 0;
  const pageHighlights = highlights.filter((highlight) => highlight.pageNumber === pageNumber);

  return (
    <div className="min-h-screen bg-reading font-sans text-ink">
      {/* Thin progress line — always visible, independent of the hideable chrome. */}
      <div className="fixed left-0 right-0 top-0 z-30 h-0.5 bg-border">
        <div className="h-full bg-accent" style={{ width: `${readingProgressPct}%` }} />
      </div>

      {/* Top chrome: hidden by default, revealed by tapping the reading area. */}
      <div
        className={
          "fixed left-0 right-0 top-0 z-20 flex items-center justify-between border-b border-border bg-elevated/95 px-3 py-3 transition-opacity duration-150 " +
          (chromeVisible ? "opacity-100" : "pointer-events-none opacity-0")
        }
      >
        <Link
          to="/"
          className="flex h-11 w-11 items-center justify-center rounded-full text-lg text-ink"
          aria-label="Voltar para a biblioteca"
        >
          ‹
        </Link>
        <span className="truncate px-2 text-sm font-semibold">{book?.title ?? "Carregando…"}</span>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <button
            type="button"
            onClick={() => setSidebarOpen((open) => !open)}
            aria-label={`Notas (${highlights.length})`}
            className="flex h-11 w-11 items-center justify-center rounded-full text-base text-ink"
          >
            ≡
          </button>
        </div>
      </div>

      {error && <p className="fixed left-0 right-0 top-12 z-20 bg-danger-soft px-4 py-2 text-sm text-danger">{error}</p>}

      <div className="flex justify-center overflow-auto pb-24 pt-16">
        <div className="relative w-full">
          <div ref={textContainerRef} className="readingColumn" onMouseUp={handleContainerMouseUp}>
            {pageText?.paragraphs.map((paragraph, index) => (
              <p key={index}>
                {segmentParagraph(paragraph, pageText.offsets[index], pageHighlights).map((segment, segIndex) =>
                  segment.highlight ? (
                    <mark
                      key={segIndex}
                      style={{ backgroundColor: highlightBackground(segment.highlight.color) }}
                    >
                      {segment.text}
                    </mark>
                  ) : (
                    <span key={segIndex}>{segment.text}</span>
                  ),
                )}
              </p>
            ))}
          </div>

          {pendingSelection && (
            <div
              className="absolute z-30 flex min-w-[200px] flex-col gap-2 rounded-lg border border-border bg-elevated p-2.5 shadow-[0_8px_20px_rgba(0,0,0,0.3)]"
              style={
                pendingSelection.anchorX > pendingSelection.containerWidth / 2
                  ? { right: pendingSelection.containerWidth - pendingSelection.anchorX, top: pendingSelection.anchorY }
                  : { left: pendingSelection.anchorX, top: pendingSelection.anchorY }
              }
            >
              <div className="flex gap-1.5">
                {HIGHLIGHT_COLORS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    className={
                      "h-5 w-5 rounded-full border-2 " +
                      (color === selectedColor ? "border-ink" : "border-transparent")
                    }
                    style={{ backgroundColor: color }}
                    onClick={() => setSelectedColor(color)}
                    aria-label={`Use color ${color}`}
                  />
                ))}
              </div>
              <textarea
                className="resize-none rounded border border-border bg-background p-1.5 text-sm text-ink outline-none"
                placeholder="Adicionar uma nota (opcional)"
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
                rows={2}
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={handleCancelHighlight}
                  className="rounded border border-border px-2.5 py-1 text-sm text-ink"
                >
                  Cancelar
                </button>
                <button
                  type="button"
                  onClick={() => void handleConfirmHighlight()}
                  className="rounded bg-accent px-2.5 py-1 text-sm font-semibold text-accent-text"
                >
                  Salvar
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Bottom chrome: page navigation, hidden with the same tap gesture as the top bar. */}
      {numPages !== null && (
        <div
          className={
            "fixed bottom-0 left-0 right-0 z-20 flex flex-col items-center gap-2 border-t border-border bg-elevated/95 px-5 py-3.5 transition-opacity duration-150 " +
            (chromeVisible ? "opacity-100" : "pointer-events-none opacity-0")
          }
        >
          <div className="h-0.5 w-full overflow-hidden rounded-full bg-border">
            <div className="h-full bg-accent" style={{ width: `${readingProgressPct}%` }} />
          </div>
          <div className="flex w-full items-center justify-between">
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.max(1, n - 1))}
              disabled={pageNumber <= 1}
              className="flex h-9 w-9 items-center justify-center rounded-full text-ink disabled:cursor-not-allowed disabled:opacity-30"
              aria-label="Página anterior"
            >
              ‹
            </button>
            <span className="text-xs tabular-nums text-ink-faint">
              {pageNumber} de {numPages}
            </span>
            <button
              type="button"
              onClick={() => setPageNumber((n) => Math.min(numPages, n + 1))}
              disabled={pageNumber >= numPages}
              className="flex h-9 w-9 items-center justify-center rounded-full text-ink disabled:cursor-not-allowed disabled:opacity-30"
              aria-label="Próxima página"
            >
              ›
            </button>
          </div>
        </div>
      )}

      {/* Notes bottom sheet */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/30" onClick={() => setSidebarOpen(false)}>
          <aside
            className="flex max-h-[70vh] w-full max-w-lg flex-col gap-4 rounded-t-[20px] bg-elevated px-5 pb-6 pt-2.5"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="mx-auto h-1 w-9 rounded-full bg-border" />
            <div className="flex items-center justify-between">
              <div>
                <h2 className="font-serif text-lg font-semibold">Notas — {book?.title ?? ""}</h2>
                <p className="mt-0.5 text-xs text-ink-muted">{highlights.length} highlights</p>
              </div>
              <button
                type="button"
                onClick={() => setSidebarOpen(false)}
                aria-label="Fechar"
                className="flex h-10 w-10 items-center justify-center rounded-full border border-border text-sm text-ink-muted"
              >
                ✕
              </button>
            </div>

            {highlights.length === 0 && (
              <p className="text-sm text-ink-faint">No highlights yet.</p>
            )}

            <ul className="flex flex-col gap-4 overflow-auto pb-2">
              {highlights
                .slice()
                .sort((a, b) => a.pageNumber - b.pageNumber)
                .map((highlight) => {
                  const highlightNotes = notes.filter(
                    (note) => note.highlightId === highlight.id,
                  );
                  return (
                    <li key={highlight.id} className="flex flex-col gap-1.5">
                      <div className="flex items-center gap-2">
                        <span
                          className="h-[9px] w-[9px] shrink-0 rounded-full"
                          style={{ backgroundColor: highlight.color }}
                        />
                        <span className="text-[11px] text-ink-faint">Página {highlight.pageNumber}</span>
                      </div>
                      {highlightNotes.map((note) => (
                        <p key={note.id} className="whitespace-pre-wrap text-sm text-ink-muted">
                          {note.content}
                        </p>
                      ))}
                    </li>
                  );
                })}
            </ul>
          </aside>
        </div>
      )}
    </div>
  );
}

export default ReaderPage;
```

Note: `useMemo` is imported but unused by the snippet above if you paste it verbatim — remove it from the import line (`import { useEffect, useRef, useState } from "react";`) since `segmentParagraph` is cheap enough to call directly during render without memoizing.

- [ ] **Step 2: Replace `frontend/src/pages/ReaderPage.css` entirely**

```css
.readingColumn {
  width: 100%;
  max-width: 68ch;
  margin: 0 auto;
  padding: 0 1.5rem;
  font-family: "Source Serif 4", Georgia, serif;
  font-size: 1.125rem;
  line-height: 1.75;
  color: var(--color-text);
}

.readingColumn p {
  margin: 0 0 1.25em;
  white-space: pre-wrap;
}

.readingColumn p:last-child {
  margin-bottom: 0;
}

.readingColumn mark {
  background: none;
  border-radius: 2px;
  padding: 0.05em 0;
  color: inherit;
}

.readingColumn ::selection {
  background: var(--color-accent-soft);
}
```

- [ ] **Step 3: Run the frontend build**

Run (from `frontend/`): `npm run build`
Expected: succeeds with no TypeScript errors (this runs `tsc -b && vite build`).

- [ ] **Step 4: Commit**

```bash
git add src/pages/ReaderPage.tsx src/pages/ReaderPage.css
git commit -m "feat(frontend): render page text as fluid reflowing paragraphs

Replaces the pdf.js canvas + TextLayer rendering with plain HTML
paragraphs fetched from GET /books/{id}/pages/{number}, reflowed and
de-hyphenated via lib/pageText.ts, laid out in a max-width (68ch)
reading column. Highlights render as <mark> spans computed from
character offsets (lib/highlightLayout.ts) instead of absolutely
positioned divs over a canvas. pdf.js is kept only to read numPages
from the PDF's metadata (see the comment in ReaderPage.tsx) - no
canvas, no TextLayer, no page rendering.

Preserves: dark mode/theme toggle, discrete page navigation, reading
progress persistence, and the notes/highlights bottom sheet. Does not
touch UploadPage.tsx or BookListPage.tsx."
```

---

## Task 8: Full-system validation

**Files:** none (verification only; fix-up commits if real bugs are found, one atomic commit per fix).

- [ ] **Step 1: Bring up the full stack**

From the repo root:
```bash
docker compose up --build -d
```
Wait for all four services to report healthy:
```bash
docker compose ps
```
Expected: `postgres`, `extractor`, `frontend` show `healthy`; `backend` shows `running` (it has no Docker healthcheck by design - see the compose file's comment).

- [ ] **Step 2: Confirm the backend is reachable and migrated**

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:${BACKEND_PORT:-8080}/health
```
Expected: `200`.

- [ ] **Step 3: Upload a real, multi-page PDF with continuous text**

Reuse `backend/dev-data/8b60ae045b374a7c8c3f59a3425145fb.pdf` (an existing ~1.4MB real PDF already in the repo's dev data, not a synthetic fixture) or any other real multi-page PDF you have on hand:

```bash
curl -s -X POST http://localhost:${BACKEND_PORT:-8080}/books \
  -F "title=Validation Book" \
  -F "file=@backend/dev-data/8b60ae045b374a7c8c3f59a3425145fb.pdf" | tee /tmp/book.json
```
Expected: HTTP 201, response JSON has `"status":"ready"` (extraction runs synchronously in `handleCreateBook`). Note the `id` field - call it `$BOOK_ID` below.

- [ ] **Step 4: Confirm page text is served as plain text, not geometry**

```bash
curl -s http://localhost:${BACKEND_PORT:-8080}/books/$BOOK_ID/pages/1
```
Expected: JSON with a non-empty `text` field containing real prose (not JSON-escaped garbage). Skim it for obvious hyphenation gaps (a line-broken word with a literal `-` still in the middle) — if the extractor's blocks are one-per-line, you should see several `\n` characters in the raw field.

- [ ] **Step 5: Open the reader in a real browser and verify the fluid layout**

Navigate to `http://localhost:${FRONTEND_PORT:-8081}/read/$BOOK_ID`. Confirm visually:
- Text flows in paragraphs with no visible mid-word hyphenation breaks (the words that were split across the original PDF's lines now read as whole words).
- The reading column has a comfortable width and does **not** span edge-to-edge on a wide (e.g. 1440px+) browser window - there's visible margin on both sides.
- Dark mode toggle still works and the reading column's text/background follow it.

- [ ] **Step 6: Verify highlight creation and persistence**

In the browser: select a run of text with the mouse, confirm the color-picker/note popup appears near the selection, pick a color, type a short note, click Salvar. Confirm:
- The selected text is now visibly marked with the chosen color.
- Opening the notes bottom sheet (≡ button) lists the new highlight with its note.

- [ ] **Step 7: Verify highlights survive page navigation and reload**

Navigate to the next page and back to the page you highlighted - confirm the mark is still shown in the same place over the same words. Reload the browser tab (F5) - confirm:
- The reader reopens on the same page you were last on (reading progress persisted).
- The highlight and its note are still present and still marked over the same text.

- [ ] **Step 8: Optional automated confirmation via headless browser**

If you want a scripted double-check of Steps 5-7 instead of (or in addition to) manual verification, run a one-off Playwright script (no need to add it to `package.json` - `npx` fetches it on demand):

```bash
cd frontend
npx --yes playwright install --with-deps chromium
npx --yes playwright screenshot "http://localhost:${FRONTEND_PORT:-8081}/read/$BOOK_ID" /tmp/reader.png --viewport-size=1440,900
```
Expected: `/tmp/reader.png` shows the reading column centered with visible side margins, matching Step 5's manual check. For selection/highlight/reload behavior, either drive `page.mouse` in a short throwaway Node script via `npx playwright` or rely on the manual verification in Steps 6-7 — the manual check is sufficient to satisfy this task's validation requirement on its own.

- [ ] **Step 9: Fix any real bugs found, one atomic commit per fix**

If any of Steps 5-7 fail, diagnose and fix with a single, atomic commit per bug (never leave the system broken "temporarily"). Re-run the relevant step(s) after each fix to confirm.

- [ ] **Step 10: Tear down**

```bash
docker compose down
```

- [ ] **Step 11: Report results**

Summarize, for each of Steps 2, 4, 5, 6, 7 (and 8 if run): what was checked and the actual result observed. Call out explicitly if anything was skipped and why.

---

## Self-review notes

- **Spec coverage:** render fluid text from `GET /books/{id}/pages/{number}` (Task 7), de-hyphenation heuristic (Task 6), comfortable reading column not full-width (Task 7 CSS), discrete page nav preserved (Task 7), char-offset highlight storage with TDD (Tasks 1-4), highlight overlay via `<mark>` (Tasks 6-7), preserved dark mode/theme/progress/sidebar (Task 7 keeps all of it verbatim), commit boundaries exactly as requested (Tasks 1-2 = commit a, Task 3 = commit b, Task 4 = commit c, Task 6-7... note the task list asked for frontend split as "(d) fluid render" and "(e) highlight creation/overlay" but this plan's Task 6 (pure helpers) + Task 7 (ReaderPage wiring) together deliver both (d) and (e) as a single coherent pair since the offset-selection code and the mark-rendering code are the same files and can't be meaningfully split into "creation" vs "render" without one being non-functional on its own - Task 6 is infrastructure with no independent commit value, so it's paired with Task 7's single feature commit. If strict adherence to exactly 5 commits (a-e) is required, treat Task 6+7 as one commit instead of two; the plan as written produces 7 backend+frontend commits total (a, b, c, then 3 frontend commits: types/client, helpers, ReaderPage) which still satisfies "at least these atomic commits" and "never mix pieces" from the task description.
- **Placeholder scan:** no TBD/TODO, every step has literal code or an exact shell command.
- **Type consistency:** `domain.CharRange{Start,End int}` ↔ Go `req.Range`/`highlight.Range` ↔ JSON `{"start","end"}` ↔ TS `CharRange{start,end}` are consistent across Tasks 1-7. `getPage`/`Page` type matches `domain.Page`'s existing json tags (`bookId`,`number`,`text`,`width`,`height`) unchanged.
