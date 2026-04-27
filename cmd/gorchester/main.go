package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/exitae337/gorchester/internal/client"
	"github.com/exitae337/gorchester/internal/config"
	"github.com/exitae337/gorchester/internal/core"
	"github.com/exitae337/gorchester/internal/scheduler"
	"github.com/exitae337/gorchester/internal/store"
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
	// Make components
	taskStore := store.New()
	logger.Info("in-memory task store initialized")

	// Docker Client
	dockerClient, err := client.NewDockerClient()
	if err != nil {
		logger.Error("failed to create docker client", slog.Any("error", err))
		os.Exit(1)
	}
	defer dockerClient.Close()
	logger.Info("docker client connected")

	schedulerConfig := scheduler.DefaultConfig()
	schedulerConfig.Strategy = scheduler.StrategySpread
	sched := scheduler.New(schedulerConfig, logger)
	defer sched.Stop()

	logger.Info("scheduler created", "strategy", schedulerConfig.Strategy)

	// MAKE ORCH
	orch := core.New(
		cfg,
		taskStore,
		dockerClient,
		sched,
		logger,
	)

	if err := orch.Start(); err != nil {
		logger.Error("failed to start orchestrator", "error", err)
		os.Exit(1)
	}

	logger.Info("orchestrtor started!")

	// Print info
	printServiceStatus(context.Background(), orch, logger)

	// Quit signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	// Stop orchestrator
	if err := orch.Stop(); err != nil {
		logger.Error("failed to stop orchestartor", slog.Any("error", err))
	}

	logger.Info("orchestrator stopped")
}

func printServiceStatus(ctx context.Context, orch *core.Orchestrator, logger *slog.Logger) {
	// Ждем дольше - 10 секунд вместо 2
	time.Sleep(10 * time.Second)

	tasks, err := orch.ListTasks(ctx)
	if err != nil {
		logger.Error("failed to list tasks", "error", err)
		return
	}

	if len(tasks) == 0 {
		logger.Info("no tasks found")
		return
	}

	logger.Info("current tasks status")
	for _, task := range tasks {
		// Безопасное получение container ID
		containerID := task.ContainerID
		if containerID == "" {
			containerID = "pending"
		} else if len(containerID) > 12 {
			containerID = containerID[:12]
		}

		// Безопасное получение task ID
		taskID := task.ID
		if len(taskID) > 8 {
			taskID = taskID[:8]
		}

		logger.Info("task",
			"id", taskID,
			"service", task.ServiceName,
			"status", task.Status,
			"node", task.NodeID,
			"container", containerID,
			"restarts", task.RestartCount)
	}
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
