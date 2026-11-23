// Package service runs the common health server for long-lived processes.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"go.opentelemetry.io/otel"
	"karun.tech/jobagent/internal/platform/langfuse"
	"karun.tech/jobagent/internal/platform/observability"
)

type pinger interface {
	PingContext(context.Context) error
}

func healthHandler(db pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "dependency unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// Run serves liveness and database readiness checks until termination.
func Run(defaultAddress string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()

	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		provider, err := langfuse.NewProvider(context.Background(), endpoint)
		if err != nil {
			observability.Logger(os.Stderr).Warn("telemetry_setup_failed")
		} else {
			otel.SetTracerProvider(provider)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := provider.Shutdown(ctx); err != nil {
					observability.Logger(os.Stderr).Warn("telemetry_shutdown_failed")
				}
			}()
		}
	}

	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = defaultAddress
	}
	server := &http.Server{
		Addr:              address,
		Handler:           observability.TraceHandler(healthHandler(db)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe() }()
	select {
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve health endpoints: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown health server: %w", err)
		}
		err := <-serveResult
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve health endpoints: %w", err)
		}
		observability.Logger(os.Stdout).Info("shutdown_complete")
		return nil
	}
}
