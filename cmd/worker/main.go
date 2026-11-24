package main

import (
	"log/slog"
	"os"

	"karun.tech/jobagent/internal/platform/observability"
	"karun.tech/jobagent/internal/platform/service"
)

func main() {
	slog.SetDefault(observability.Logger(os.Stderr))
	if err := service.Run("127.0.0.1:8081", nil); err != nil {
		slog.Error("worker_stopped", "error_class", "service_failure")
		os.Exit(1)
	}
}
