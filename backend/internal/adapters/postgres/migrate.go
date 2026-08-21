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
