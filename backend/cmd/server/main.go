// Command server wires the pdf-reader backend's concrete adapters to its
// HTTP API and starts listening. It contains no orchestration logic itself
// - that lives in internal/adapters/httpserver.
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"pdf-reader/backend/internal/adapters/filestorage"
	"pdf-reader/backend/internal/adapters/httpextractor"
	"pdf-reader/backend/internal/adapters/httpserver"
	"pdf-reader/backend/internal/adapters/postgres"
	"pdf-reader/backend/migrations"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("pinging database: %v", err)
	}

	if err := postgres.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		log.Fatalf("applying migrations: %v", err)
	}

	extractorURL := os.Getenv("EXTRACTOR_URL")
	if extractorURL == "" {
		extractorURL = "http://localhost:8000"
	}

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./data"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := httpserver.NewServer(
		postgres.NewBookRepository(db),
		postgres.NewPageRepository(db),
		postgres.NewHighlightRepository(db),
		postgres.NewNoteRepository(db),
		postgres.NewReadingProgressRepository(db),
		httpextractor.NewHTTPTextExtractor(extractorURL, nil),
		filestorage.NewFileSystemStorage(storageDir),
	)

	log.Printf("listening on :%s", port)
	httpServer := &http.Server{Addr: ":" + port, Handler: server,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 100 * time.Second,
		WriteTimeout: 200 * time.Second, IdleTimeout: 60 * time.Second}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
