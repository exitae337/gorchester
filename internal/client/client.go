package client

import (
	"context"
	"time"

	"github.com/exitae337/gorchester/internal/config"
)

// Interafce for Docker Client -> contract
type ContainerManager interface {
	// Create container
	CreateContainer(ctx context.Context, service *config.ServiceConfig, taskID string) (string, error)
	// Start container by ID
	StartContainer(ctx context.Context, containerID string) error
	// Stop container by ID
	StopContainer(ctx context.Context, containerID string) error
	// Delete container by ID
	RemoveContainer(ctx context.Context, containerID string) error
	// Container status
	GetConatinerStatus(ctx context.Context, containerID string) (string, error)
	// List all containers by filter (or all)
	ListContainers(ctx context.Context, filters map[string]string) ([]DockerContainer, error)
	// Download image for container
	PullImage(ctx context.Context, image string) error
	// Image exists
	imageExists(ctx context.Context, image string) (bool, error)
}

// Docker container struct
type DockerContainer struct {
	ID      string            `json:"id"`         // Container ID
	Name    string            `json:"name"`       // Container name
	Image   string            `json:"image"`      // Container image (from docker-hub or file)
	Status  string            `json:"status"`     // Container status
	State   string            `json:"state"`      // Container state
	Created time.Time         `json:"created_at"` // Created at timeStamp
	Labels  map[string]string `json:"labels"`     // Container labels
}

// Docker container Metrics
type DockerContainerMetrics struct {
	ContainerID string    `json:"container_id"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage int64     `json:"memory_usage"`
	MemoryLimit int64     `json:"memory_limit"`
	Timestamp   time.Time `json:"current_timestamp"`
}
