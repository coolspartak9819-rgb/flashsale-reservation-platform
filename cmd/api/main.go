package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/httpapi"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/metrics"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/outbox"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	serviceMetrics := &metrics.Metrics{}
	var repository store.ReservationStore = store.New()
	var db *sql.DB
	var cancel context.CancelFunc
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		var err error
		db, err = sql.Open("postgres", dsn)
		must(err)
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
		must(db.PingContext(context.Background()))
		must(runMigrations(db))
		repository = store.NewPostgresStore(db)
		defer db.Close()
		logger.Info("postgres persistence enabled")
		if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
			nc, err := nats.Connect(natsURL)
			must(err)
			js, err := nc.JetStream()
			must(err)
			_, streamErr := js.AddStream(&nats.StreamConfig{Name: "FLASHSALE", Subjects: []string{"flashsale.events"}})
			if streamErr != nil {
				_, streamErr = js.StreamInfo("FLASHSALE")
			}
			must(streamErr)
			go outbox.NewPublisher(repository.(*store.PostgresStore), js, logger).Run(ctx)
			defer nc.Close()
			logger.Info("nats outbox publisher enabled")
		}
	}
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: httpapi.New(repository, serviceMetrics), ReadHeaderTimeout: 5 * time.Second}
	logger.Info("flash sale API started", "addr", server.Addr)
	if expirer, ok := repository.(interface{ ExpireReservations(time.Time) (int, error) }); ok {
		go expireLoop(expirer, logger)
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func expireLoop(expirer interface{ ExpireReservations(time.Time) (int, error) }, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		count, err := expirer.ExpireReservations(now.UTC())
		if err != nil {
			logger.Error("reservation expiration failed", "error", err)
			continue
		}
		if count > 0 {
			logger.Info("reservations expired", "count", count)
		}
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
