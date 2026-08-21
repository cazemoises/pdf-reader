// Package postgres_test contains integration tests for ApplyMigrations.
//
// This test runs against a real Postgres database - see
// book_repository_test.go's package doc comment for local setup
// instructions. Skipped if DATABASE_URL is unset.
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
