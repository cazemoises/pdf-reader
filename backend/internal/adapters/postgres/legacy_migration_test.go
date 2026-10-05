package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/migrations"
	"testing"
	"time"
)

func TestExtractionMigrationPreservesLegacyPagesHighlightsAndRollback(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("legacy_extract_%d", time.Now().UnixNano())
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	if _, err = db.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	names, _ := fs.Glob(migrations.FS, "*.sql")
	for _, name := range names {
		if name == "0008_page_extraction.sql" {
			continue
		}
		data, _ := fs.ReadFile(migrations.FS, name)
		if _, err = db.ExecContext(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	original := "Primeiro parágrafo.\n\nSegundo parágrafo."
	if _, err = db.ExecContext(ctx, `INSERT INTO books VALUES ('legacy','Legacy','legacy.pdf','ready',now(),now());
 INSERT INTO highlights VALUES ('mark','legacy',1,'yellow',now(),0,8);`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO pages (book_id,number,text,width,height) VALUES ('legacy',1,$1,600,800)`, original); err != nil {
		t.Fatal(err)
	}
	migration, _ := fs.ReadFile(migrations.FS, "0008_page_extraction.sql")
	for i := 0; i < 2; i++ {
		if _, err = db.ExecContext(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := postgres.NewPageRepository(db).FindByBookIDAndNumber(ctx, "legacy", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Text != original || len(page.Extraction) != 0 {
		t.Fatalf("legacy page changed: %+v", page)
	}
	var start, end int
	if err = db.QueryRowContext(ctx, `SELECT start_offset,end_offset FROM highlights WHERE id='mark'`).Scan(&start, &end); err != nil {
		t.Fatal(err)
	}
	if start != 0 || end != 8 {
		t.Fatal("highlight offsets changed")
	}
	// Old application's SELECT/INSERT remain valid after rollback; nullable additive column is ignored.
	var rollbackText string
	if err = db.QueryRowContext(ctx, `SELECT text FROM pages WHERE book_id='legacy' AND number=1`).Scan(&rollbackText); err != nil || rollbackText != original {
		t.Fatal("old SELECT broken", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO pages (book_id,number,text,width,height) VALUES ('legacy',2,'Old writer',600,800)`); err != nil {
		t.Fatal("old INSERT broken", err)
	}
}
