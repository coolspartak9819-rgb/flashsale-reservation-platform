package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/httpapi"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
	_ "github.com/lib/pq"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var repository store.ReservationStore = store.New()
	var db *sql.DB
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		var err error
		db, err = sql.Open("postgres", dsn)
		must(err)
		must(db.PingContext(context.Background()))
		must(runMigrations(db))
		repository = store.NewPostgresStore(db)
		defer db.Close()
		logger.Info("postgres persistence enabled")
	}
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: httpapi.New(repository), ReadHeaderTimeout: 5 * time.Second}
	logger.Info("flash sale API started", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func runMigrations(db *sql.DB) error {
	contents, err := os.ReadFile("migrations/001_schema.sql")
	if err != nil {
		return err
	}
	_, err = db.Exec(string(contents))
	return err
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
