package main

import (
	"log/slog"
	"os"

	"karun.tech/jobagent/internal/platform/service"
)

func main() {
	if err := service.Run(":8080"); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}
