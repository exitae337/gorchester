package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/exitae337/gorchester/internal/config"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	// Loading configuration file for orchestration process
	cfg := config.MustLoad()
	// Init Logger
	logger := setupLogger(cfg.Env)
	if logger == nil {
		log.Fatalf("env string must be: local, dev or prod")
	}
	logger.Info("starting orchestrator", slog.String("env", cfg.Env))
	logger.Debug("debug messages are enabled")
	// TODO: Start orchestrator
}

func setupLogger(env string) *slog.Logger {
	var logger *slog.Logger
	switch env {
	case envLocal:
		logger = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envDev:
		logger = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		logger = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	default:
		logger = nil
	}
	return logger
}
