# Docker Compose Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a root-level `docker-compose.yml` that brings up postgres, backend, extractor, and frontend with `docker compose up` and no manual steps beyond optional `.env` overrides, closing the last gap before declaring the product "ready to deploy."

**Architecture:** Two independently-shippable changes. (1) A backend auto-migrate feature (new `migrations` package with `go:embed`, plus `postgres.ApplyMigrations`, wired into `main.go`) so the database schema is never a manual step. (2) The compose file itself, plus an nginx reverse-proxy config for the frontend container (the frontend calls the API via relative paths like `/books` — proxying is required, not just an env var, because there's no existing `VITE_API_URL` to configure) and doc-comment fixes to the two Dockerfiles that are now stale.

**Tech Stack:** Go 1.26 (stdlib `embed`, `database/sql`), Docker Compose v2, nginx (official image's built-in template/proxy behavior not needed — static conf is enough), Postgres 16.

**Spec:** User-provided task description (see conversation) plus findings from codebase investigation below. The investigation overrides two incorrect assumptions in the original spec:
- `EXTRACTOR_URL` is **already** a configurable env var read in `main.go` (backend/cmd/server/main.go:36-39) and injected into `httpextractor.NewHTTPTextExtractor` at the composition root. **No code change needed for this part.**
- `frontend/package-lock.json` is **already committed** (verified via `git show HEAD:frontend/package-lock.json`). **No commit needed for this part.**
- The frontend has no `VITE_API_URL` anywhere in `frontend/src` — it calls the backend via bare relative paths (`fetch("/books")` etc., see `frontend/src/api/client.ts`) and relies on a Vite **dev-server proxy** (`frontend/vite.config.ts`: `/books` → `http://localhost:8080`) that does not exist in the built nginx image. The compose file must reproduce that proxy at the nginx layer, or `/books` calls from the built frontend will 404 against nginx itself. This plan uses a static `frontend/nginx.conf` proxying `/books` and `/health` to `http://backend:8080` inside the compose network, mirroring the existing dev-time proxy shape instead of introducing a new `VITE_API_URL` mechanism (which would need build-time Vite env baking or a runtime envsubst script — more moving parts for the same result, and touches frontend application code + its own test suite for no behavioral gain over a proxy).
- `backend/internal/adapters/postgres/book_repository_test.go`'s doc comment (lines 7-15) **already documents** the intended compose design: credentials `pdfreader`/`pdfreader`/`pdfreader`, and **no published host port** for postgres. This plan follows that pre-existing documentation exactly so the test comment stays true.
- Migrations (`backend/migrations/0001..0005*.sql`) are all idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`), confirmed by reading all five files. That makes "run every file every startup" a safe, simple auto-migrate strategy — no migration-tracking table needed.
- `backend/internal/adapters/httpserver/server_test.go:75` already hand-lists the 5 migration filenames and applies them the same way tests always have. This plan does not touch that test (out of scope, lower risk) — it only adds new production code alongside it.

## Global Constraints

- No TDD skip: any new Go logic gets a failing test first (RED) before implementation (GREEN). Composition-root wiring in `main()` has no existing test precedent in this codebase (zero tests currently target `cmd/server`) and is not being retrofitted with one here — that's a bigger refactor (extracting a testable `run()`) than this task calls for. State this explicitly rather than faking coverage.
- No new external dependencies. Auto-migrate uses only `database/sql`, `embed`, `io/fs`, `sort` — all stdlib.
- Hexagonal boundaries: `internal/domain` stays untouched. The new `ApplyMigrations` function lives in `internal/adapters/postgres` (it's Postgres-specific bootstrap SQL execution) and takes an `fs.FS` parameter — `main.go` (composition root) supplies the concrete `embed.FS` from the new `migrations` package. No `os.Getenv` inside adapters.
- Commit atomicity: one commit for the auto-migrate feature (Go production code, TDD), one commit for the compose file + supporting infra/doc files. Do not mix the two.
- Postgres credentials default to `pdfreader`/`pdfreader`/`pdfreader`, matching `book_repository_test.go`'s pre-existing doc comment.
- Postgres service publishes **no host port** (matches the same doc comment — host machine already has an unrelated `postgres:16-alpine` container bound to `5432`, confirmed via `docker ps`, so this also avoids a real port collision on this machine).
- Environment variables are all optional overrides with sane defaults inlined in `docker-compose.yml` (`${VAR:-default}` syntax) — `.env` is documented via `.env.example` but never required.

---

### Task 1: Auto-migrate backend schema on startup

**Files:**
- Create: `backend/migrations/migrations.go`
- Create: `backend/internal/adapters/postgres/migrate_test.go`
- Create: `backend/internal/adapters/postgres/migrate.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: `migrations.FS` (`embed.FS`, package `pdf-reader/backend/migrations`) — the embedded `*.sql` files.
- Produces: `postgres.ApplyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error` (package `pdf-reader/backend/internal/adapters/postgres`) — reads every `*.sql` file in `fsys`, sorts filenames lexically (zero-padded numeric prefixes sort correctly), execs each in order against `db`. Safe to call every startup because migration files are idempotent.
- Consumes (in `main.go`): both of the above, called after `db.Ping()` succeeds and before constructing repositories.

- [ ] **Step 1: Create the embeddable migrations package**

```go
// backend/migrations/migrations.go

// Package migrations embeds the SQL migration files so the compiled
// server binary can apply them at startup without needing the source
// tree on disk (the production Docker image is distroless).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Write the failing test for ApplyMigrations**

```go
// backend/internal/adapters/postgres/migrate_test.go
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/migrations"
)

// TestApplyMigrations_CreatesAllExpectedTables runs against a real
// Postgres database - see book_repository_test.go's package doc comment
// for local setup instructions. Skipped if DATABASE_URL is unset.
func TestApplyMigrations_CreatesAllExpectedTables(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("pinging database: %v", err)
	}

	if err := postgres.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("ApplyMigrations: unexpected error: %v", err)
	}
	// Migration files use CREATE TABLE/INDEX IF NOT EXISTS, so a second
	// run against the same database must stay a no-op, not an error.
	if err := postgres.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("ApplyMigrations (second run): unexpected error: %v", err)
	}

	for _, table := range []string{"books", "pages", "highlights", "notes", "reading_progress"} {
		var exists bool
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("checking table %q: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created by ApplyMigrations", table)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails (RED)**

Run: `go build ./... && go vet ./...` inside `backend/` — expect a compile failure: `undefined: postgres.ApplyMigrations` (the function doesn't exist yet). This is the RED signal; a full `go test` run isn't needed yet since it won't even compile.

- [ ] **Step 4: Implement ApplyMigrations (GREEN)**

```go
// backend/internal/adapters/postgres/migrate.go
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// ApplyMigrations executes every *.sql file found in fsys against db, in
// lexical filename order (migration files are zero-padded, e.g.
// 0001_create_books.sql, so lexical order matches intended order). Each
// file is expected to be idempotent (CREATE TABLE/INDEX IF NOT EXISTS),
// so this is safe to call on every process startup.
func ApplyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("postgres: reading migrations: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("postgres: reading migration %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("postgres: applying migration %s: %w", name, err)
		}
	}

	return nil
}
```

- [ ] **Step 5: Wire ApplyMigrations into main.go**

In `backend/cmd/server/main.go`, add the import `"pdf-reader/backend/migrations"` and call `ApplyMigrations` right after the existing `db.Ping()` check (around line 34), before `extractorURL := os.Getenv(...)`:

```go
	if err := db.Ping(); err != nil {
		log.Fatalf("pinging database: %v", err)
	}

	if err := postgres.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		log.Fatalf("applying migrations: %v", err)
	}
```

This needs `"context"` added to the existing import block.

- [ ] **Step 6: Run tests to verify GREEN**

Run: `go build ./... && go vet ./...` inside `backend/` — expect success (proves `main.go` still compiles with the new wiring).
Run: `go test ./...` inside `backend/` — expect all DB-backed tests to skip (no `DATABASE_URL` set), everything else to pass.
Then, with a real Postgres reachable (e.g. start one via `docker run --rm -d -e POSTGRES_USER=pdfreader -e POSTGRES_PASSWORD=pdfreader -e POSTGRES_DB=pdfreader -p 15432:5432 postgres:16-alpine` and `export DATABASE_URL=postgres://pdfreader:pdfreader@localhost:15432/pdfreader?sslmode=disable`), run `go test ./internal/adapters/postgres/... -run TestApplyMigrations -v` — expect PASS, confirming the new test actually exercises real Postgres rather than skipping.

- [ ] **Step 7: Commit**

```bash
git add backend/migrations/migrations.go backend/internal/adapters/postgres/migrate.go backend/internal/adapters/postgres/migrate_test.go backend/cmd/server/main.go
git commit -m "feat(backend): auto-apply database migrations on startup"
```

---

### Task 2: docker-compose.yml and supporting deploy infra

**Files:**
- Create: `docker-compose.yml` (repo root, sibling to `backend/`, `extractor/`, `frontend/`)
- Create: `.env.example` (repo root)
- Create: `frontend/nginx.conf`
- Modify: `frontend/Dockerfile`
- Modify: `backend/Dockerfile`

**Interfaces:**
- Consumes: `backend/Dockerfile` (existing, builds `cmd/server` — now buildable, unmodified except the comment), `extractor/Dockerfile` (existing, `EXPOSE 8000`), `frontend/Dockerfile` (existing, `EXPOSE 80`, nginx base image).
- Produces: a running stack on `docker compose up` — frontend on `http://localhost:${FRONTEND_PORT:-8081}`, backend API on `http://localhost:${BACKEND_PORT:-8080}`, extractor on `http://localhost:${EXTRACTOR_PORT:-8000}`, postgres reachable only inside the compose network as `postgres:5432`.

- [ ] **Step 1: Write frontend/nginx.conf**

```nginx
# frontend/nginx.conf
#
# Replaces the nginx base image's default server block. The built
# frontend calls the backend via bare relative paths (see
# frontend/src/api/client.ts, e.g. fetch("/books")) instead of an
# absolute URL, mirroring the Vite dev-server proxy in
# frontend/vite.config.ts. This config reproduces that proxy so the
# same frontend code works unmodified in the compose network.
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    location /books {
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /health {
        proxy_pass http://backend:8080;
    }

    location / {
        try_files $uri /index.html;
    }
}
```

- [ ] **Step 2: Update frontend/Dockerfile**

Replace the stale top-of-file comment (package-lock.json is committed now) and add the nginx config copy:

```dockerfile
FROM node:20-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost/ >/dev/null || exit 1
```

(Drop the `*` glob on `package-lock.json` too — it's unconditionally present now, so `npm ci` should always see it rather than silently building without a lockfile if it's ever deleted.)

- [ ] **Step 3: Fix stale comment in backend/Dockerfile**

Remove the top comment block claiming `cmd/server/main.go` doesn't exist yet (it does, and has since the task-priority-server work):

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
```

- [ ] **Step 4: Write docker-compose.yml**

```yaml
# docker-compose.yml
#
# Brings up the full pdf-reader stack: postgres, backend (Go), extractor
# (Python/FastAPI), frontend (nginx-served React build). All env vars
# have sane defaults inlined below; override any of them via a .env file
# (see .env.example) if needed. No manual steps required after `docker
# compose up` - the backend applies its own database migrations on
# startup (see backend/internal/adapters/postgres/migrate.go).
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-pdfreader}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-pdfreader}
      POSTGRES_DB: ${POSTGRES_DB:-pdfreader}
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-pdfreader} -d ${POSTGRES_DB:-pdfreader}"]
      interval: 5s
      timeout: 5s
      retries: 5
    # Intentionally no published host port - postgres is only reachable
    # from other services on the compose network. Matches the setup
    # documented in backend/internal/adapters/postgres/book_repository_test.go.

  extractor:
    build: ./extractor
    ports:
      - "${EXTRACTOR_PORT:-8000}:8000"

  backend:
    build: ./backend
    environment:
      DATABASE_URL: postgres://${POSTGRES_USER:-pdfreader}:${POSTGRES_PASSWORD:-pdfreader}@postgres:5432/${POSTGRES_DB:-pdfreader}?sslmode=disable
      EXTRACTOR_URL: http://extractor:8000
      PORT: 8080
      STORAGE_DIR: /data
    volumes:
      - backend-storage:/data
    ports:
      - "${BACKEND_PORT:-8080}:8080"
    depends_on:
      postgres:
        condition: service_healthy
      extractor:
        condition: service_started

  frontend:
    build: ./frontend
    ports:
      - "${FRONTEND_PORT:-8081}:80"
    depends_on:
      - backend

volumes:
  postgres-data:
  backend-storage:
```

- [ ] **Step 5: Write .env.example**

```bash
# .env.example
# Copy to .env and adjust if you need non-default values. Every
# variable already has a working default in docker-compose.yml, so a
# .env file is optional.

POSTGRES_USER=pdfreader
POSTGRES_PASSWORD=pdfreader
POSTGRES_DB=pdfreader

# Host-side ports. Change these if they collide with something already
# running on your machine.
BACKEND_PORT=8080
EXTRACTOR_PORT=8000
FRONTEND_PORT=8081
```

- [ ] **Step 6: Validate the stack actually builds and boots**

Run: `docker compose build` from the repo root — expect all three custom images to build successfully (backend compiles `cmd/server`, extractor installs its Python deps, frontend runs `npm ci && npm run build`).
Run: `docker compose up -d` — expect `postgres` to become healthy, then `backend` and `frontend` to start.
Run: `docker compose ps` — expect all 4 services `Up` (postgres `healthy`).
Run: `curl http://localhost:8080/health` — expect a 2xx response from the backend directly.
Run: `curl http://localhost:8081/books` — expect the same kind of response proxied through nginx (not a 404 from nginx's static file server), proving the `/books` proxy location works.
Run: `docker compose down -v` when done to tear down and remove the named volumes (only if this was a throwaway validation run — keep volumes if the user wants to keep exploring the running stack).

- [ ] **Step 7: Commit**

```bash
git add docker-compose.yml .env.example frontend/nginx.conf frontend/Dockerfile backend/Dockerfile
git commit -m "feat(deploy): add docker-compose stack for postgres, backend, extractor, frontend"
```

---

## Self-Review

**Spec coverage:**
- postgres service with `POSTGRES_USER/PASSWORD/DB`, named volume, healthcheck → Task 2 Step 4. ✓
- backend built from `./backend`, correct `DATABASE_URL` format (confirmed by reading `main.go` — `os.Getenv("DATABASE_URL")` passed straight to `sql.Open("postgres", dsn)`, so any valid `lib/pq` DSN works), `EXTRACTOR_URL` confirmed already configurable → Task 2 Step 4, investigation notes above. ✓
- extractor built from `./extractor`, port 8000 confirmed from Dockerfile `EXPOSE 8000` / uvicorn `--port 8000` → Task 2 Step 4. ✓
- frontend built from `./frontend`, API URL handling → resolved via nginx proxy instead of `VITE_API_URL` (justified above) → Task 2 Steps 1-2, 4. ✓
- main.go env var confirmation → done during investigation, no code change needed, documented in Spec section. ✓
- migrations auto-apply → Task 1, option (a) chosen (simple stdlib embed + exec, idempotent SQL makes it trivial and safe). ✓
- package-lock.json commit check → done during investigation, already committed, no action needed, documented in Spec section. ✓
- backend/Dockerfile stale comment → Task 2 Step 3. ✓
- Atomic commits → 2 commits total, one per task, Go production code isolated from infra/doc files. ✓
- Validation commands documented for the human/PO, not falsely claimed as already run in an unverifiable way → Task 2 Step 6 doubles as both the plan's own validation and the commands to report back; also actually runnable here since this session has both `go` and a working `docker` daemon (confirmed via `docker ps`), so these will genuinely be executed, not just prescribed.

**Placeholder scan:** No TBD/TODO markers; every code step has full file contents, not descriptions.

**Type consistency:** `postgres.ApplyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error` is defined once in Task 1 Step 4 and consumed identically in Task 1 Steps 2 (test) and 5 (main.go). `migrations.FS` (`embed.FS`, which satisfies `fs.FS`) is defined in Task 1 Step 1 and consumed the same way in both places.
