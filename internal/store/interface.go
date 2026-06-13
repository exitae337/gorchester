// Package store. Интерфейс хранилища задач.
package store

import (
	"context"

	"github.com/exitae337/gorchester/internal/types"
)

// TaskStore defines the interface for task storage
type TaskStore interface {
	// Create new Task
	Create(ctx context.Context, task *types.Task) error
	// Get Task by ID
	Get(ctx context.Context, id string) (*types.Task, error)
	// Update Task
	Update(ctx context.Context, task *types.Task) error
	// Delete Task
	Delete(ctx context.Context, id string) error

	// List all Tasks
	List(ctx context.Context) ([]*types.Task, error)
	// List all Tasks by Service ID
	ListByService(ctx context.Context, serviceName string) ([]*types.Task, error)
	// List Tasks by status
	ListByStatus(ctx context.Context, status types.TaskStatus) ([]*types.Task, error)
	// Get by Node ID
	ListByNodeID(ctx context.Context, nodeID string) ([]*types.Task, error)

	// Count all tasks
	Count(ctx context.Context) (int, error)
	// Count Tasks by service
	CountByService(ctx context.Context, serviceID string) (int, error)
	// Count by status
	CountByStatus(ctx context.Context, status types.TaskStatus) (int, error)

	// Get Task by container ID
	GetByContainerID(ctx context.Context, containerID string) (*types.Task, error)
	// Get task Stats
	TaskStats(ctx context.Context, id string) (*types.TaskStats, error)
	// Update several Tasks
	UpdateMany(ctx context.Context, tasks []types.Task) error
	// Update Task status
	UpdateStatus(ctx context.Context, id string, status types.TaskStatus) error
	// Increment Restart Counter
	IncrementRestartCounter(ctx context.Context, id string) error
}
