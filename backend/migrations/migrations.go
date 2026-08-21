// Package migrations embeds the SQL migration files so the compiled
// server binary can apply them at startup without needing the source
// tree on disk (the production Docker image is distroless).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
