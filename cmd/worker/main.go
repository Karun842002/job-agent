package main

import (
	"log/slog"
	"os"

	"karun.tech/jobagent/internal/platform/service"
)

func main() {
	if err := service.Run(":8081"); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
