package client

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/client"
)

// Docker client
type DockerClient struct {
	cli *client.Client
}

// New Docker Client -> docker API
func NewDockerClient() (*DockerClient, error) {
	const op = "client.NewDockerClient"
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker-client: %w", err)
	}
	// Check connection to Docker Daemon
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = apiClient.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker Daemon: %w, %s", err, op)
	}
	return &DockerClient{cli: apiClient}, nil
}
