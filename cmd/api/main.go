package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/httpapi"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: httpapi.New(store.New()), ReadHeaderTimeout: 5 * time.Second}
	logger.Info("flash sale API started", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
