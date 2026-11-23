package main

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"karun.tech/jobagent/internal/company"
	"karun.tech/jobagent/internal/platform/observability"
	"karun.tech/jobagent/internal/platform/service"
)

func main() {
	slog.SetDefault(observability.Logger(os.Stderr))
	if err := service.Run("127.0.0.1:8080", func(db *sql.DB) http.Handler {
		return company.Handler(company.Store{DB: db}, os.Getenv("API_BEARER_TOKEN"))
	}); err != nil {
		slog.Error("api_stopped", "error_class", "service_failure")
		os.Exit(1)
	}
}
