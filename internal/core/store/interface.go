package store

import (
	"context"
	"time"

	"github.com/exitae337/gorchester/internal/core"
)

type TaskStore interface {
	// Create new Task
	Create(ctx context.Context, task *core.Task) error
	// Get Task by ID
	Get(ctx context.Context, id string) (*core.Task, error)
	// Update Task
	Update(ctx context.Context, task *core.Task) error
	// Delete Task
	Delete(ctx context.Context, id string) error

	// List all Tasks
	List(ctx context.Context) ([]*core.Task, error)
	// List all Tasks by Service ID
	ListByService(ctx context.Context, serviceName string) ([]*core.Task, error)
	// List Tasks by status
	ListByStatus(ctx context.Context, status core.TaskStatus) ([]*core.Task, error)
	// Get by Node ID
	ListByNodeID(ctx context.Context, nodeID string) ([]*core.Task, error)

	// Count all tasks
	Count(ctx context.Context) (int, error)
	// Count Tasks by service
	CountByService(ctx context.Context, serviceID string) (int, error)
	// Count by status
	CountByStatus(ctx context.Context, status core.TaskStatus) (int, error)

	// Get Task by container ID
	GetByContainerID(ctx context.Context, containerID string) (*core.Task, error)
	// Get task Stats
	TaskStats(ctx context.Context, id string) (*TaskStats, error)
	// Update several Tasks
	UpdateMany(ctx context.Context, tasks []core.Task) error
	// Update Task status
	UpdateStatus(ctx context.Context, id string, status core.TaskStatus) error
	// Increment Restart Counter
	IncrementRestartCounter(ctx context.Context, id string) error
}

type TaskStats struct {
	Uptime        time.Duration   `json:"uptime"`
	RestartCount  int             `json:"restart_counter"`
	CPUUsage      float64         `json:"cpu_usage"`
	MemoryUsage   int64           `json:"memory_usage"`
	CurrentStatus core.TaskStatus `json:"current_status"`
}
