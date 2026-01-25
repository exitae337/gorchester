package core

import (
	"time"

	"github.com/exitae337/gorchester/internal/config"
	"github.com/exitae337/gorchester/internal/types"
)

// Status type
type TaskStatus string

// Statuses
const (
	TaskStatusPending TaskStatus = "pending" // Container created, but not started
	TaskStatusRunning TaskStatus = "running" // Container running
	TaskStatusStopped TaskStatus = "stopped" // Container stopped
	TaskStatusFailed  TaskStatus = "failed"  // Error in container running
	TaskStatusDead    TaskStatus = "dead"    // Container ended
)

// Task structure
type Task struct {
	ID            string                `json:"id"`                    // Task ID
	ServiceName   string                `json:"service_name"`          // Service name
	ContainerID   string                `json:"container_id"`          // Container's ID with service
	Status        TaskStatus            `json:"task_status"`           // Task status
	DesiredState  TaskStatus            `json:"desired_state"`         // Desired state of Task (container)
	NodeID        string                `json:"node_id"`               // Node ID
	CreatedAt     time.Time             `json:"created_at"`            // Created timestamp
	UpdatedAt     time.Time             `json:"updated_at"`            // Updated timestamp
	StartedAt     *time.Time            `json:"started_at,omitempty"`  // Task start time
	FinishedAt    *time.Time            `json:"finished_at,omitempty"` // Task finished time
	ExitCode      int                   `json:"exit_code,omitempty"`   // Task exit code
	Error         string                `json:"err,omitempty"`         // If error occurred
	RestartCount  int                   `json:"restart_counter"`       // Task restart counter
	PortMapping   []types.PortMapping   `json:"port_mapping"`          // Task port mapping
	CPUUsage      int64                 `json:"cpu_usage"`             // CPU Usage in millicores
	MemoryUsage   int64                 `json:"mem_usage"`             // Memory usage in bytes
	Labels        map[string]string     `json:"labels"`                // Meta info
	ServiceConfig *config.ServiceConfig `json:"service_config"`        // Service configuration
}

// Is task runnung
func (t *Task) IsRunning() bool {
	return t.Status == TaskStatusRunning
}

// Is task trminated
func (t *Task) IsTerminated() bool {
	return t.Status == TaskStatusStopped ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusDead
}

// Is task needs restart
func (t *Task) NeedsRestart() bool {
	return t.DesiredState == TaskStatusRunning && t.IsTerminated()
}

// Update Task status
func (t *Task) UpdateTask(newStatus TaskStatus) {
	t.Status = newStatus
	t.UpdatedAt = time.Now()

	if newStatus == TaskStatusRunning && t.StartedAt == nil {
		now := time.Now()
		t.StartedAt = &now
	}

	if t.IsTerminated() && t.FinishedAt == nil {
		now := time.Now()
		t.FinishedAt = &now
	}
}
