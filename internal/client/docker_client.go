package client

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/exitae337/gorchester/internal/config"
	"github.com/exitae337/gorchester/internal/types"
)

const (
	defaultTimeout = 30 * time.Second
	labelPrefix    = "gorchester."
)

// Docker client
type DockerClient struct {
	cli     *client.Client
	timeout time.Duration
}

// New Docker Client -> docker API
func NewDockerClient() (*DockerClient, error) {
	const op = "client.NewDockerClient"
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(defaultTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: error with create docker client: %w", op, err)
	}
	// Check connection with Docker Daemon
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("%s: failed to ping Docker Daemon: %w", op, err)
	}

	return &DockerClient{
		cli:     cli,
		timeout: defaultTimeout,
	}, nil
}

// Create Container
func (dc *DockerClient) CreateContainer(ctx context.Context, service *config.ServiceConfig, taskID string) (string, error) {
	const op = "client.CreateContainer"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	if exists, err := dc.imageExists(ctx, service.Image); err != nil {
		return "", fmt.Errorf("%s: failed to check image locally: %w", op, err)
	} else if !exists {
		if err := dc.PullImage(ctx, service.Image); err != nil {
			return "", fmt.Errorf("%s: failed to download image: %w", op, err)
		}
	}

	// Make configuration
	containerConfig := &container.Config{
		Image:        service.Image,
		Env:          convertEnvVars(service.Env),
		Cmd:          service.Command,
		ExposedPorts: createExposedPorts(service.Ports),
		Labels: map[string]string{
			"gorchester.service": service.ServiceName,
			"gorchester.task_id": taskID,
			"managed-by":         "gorchester",
		},
	}

	// Host configuration
	hostConfig := &container.HostConfig{
		PortBindings:  createPortBindings(service.Ports),
		RestartPolicy: getRestartPolicy(service.RestartPolicy),
		Resources: container.Resources{
			NanoCPUs:   int64(service.Resources.CPUMilliCores * 1_000_000),
			Memory:     service.Resources.MemoryBytes,
			MemorySwap: service.Resources.MemoryBytes,
			CpusetCpus: service.Resources.CPUSet,
			// DiskQouta...
		},
		Binds:       service.Volumes,
		NetworkMode: container.NetworkMode(service.NetworkMode),
		DNS:         service.DNS,
		ExtraHosts:  service.ExtraHosts,
	}

	// Network Config
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			service.Network: {
				NetworkID: service.Network,
			},
		},
	}

	// Make container
	resp, err := dc.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil, // Platform
		generateContainerName(service.ServiceName, taskID),
	)

	if err != nil {
		return "", fmt.Errorf("%s: error creating container: %w", op, err)
	}

	if err := dc.StartContainer(ctx, resp.ID); err != nil {
		cleanUpCtx, cleanUpCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanUpCancel()
		dc.cli.ContainerRemove(cleanUpCtx, resp.ID, container.RemoveOptions{})
		return "", fmt.Errorf("%s: failed to start container: %w", op, err)
	}

	return resp.ID, nil
}

// Start container
func (dc *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	const op = "client.StartConatiner"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	if err := dc.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("%s: error starting container: %s -> err: %w", op, containerID[:12], err)
	}

	return nil
}

// PullImage загружает образ Docker
func (dc *DockerClient) PullImage(ctx context.Context, im string) error {
	const op = "client.PullImage"

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Big timeout for downloading image
	defer cancel()

	reader, err := dc.cli.ImagePull(ctx, im, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("%s: failed to pull image %s: %w", op, im, err)
	}
	defer reader.Close()

	// Read output
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err != nil {
			break
		}
		// Logging process with JSON output
	}

	return nil
}

// ImageExists - check if image exists locally
func (dc *DockerClient) imageExists(ctx context.Context, image string) (bool, error) {
	const op = "client.imageExists"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	_, err := dc.cli.ImageInspect(ctx, image)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("%s: failed to inspect image %s: %w", op, image, err)
	}

	return true, nil
}

// Util funcs

func convertEnvVars(envVars []string) []string {
	result := make([]string, 0, len(envVars))
	result = append(result, envVars...)
	return result
}

func createExposedPorts(ports []types.PortMapping) nat.PortSet {
	portSet := make(nat.PortSet)
	for _, port := range ports {
		containerPort := nat.Port(fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
		portSet[containerPort] = struct{}{}
	}
	return portSet
}

func createPortBindings(ports []types.PortMapping) nat.PortMap {
	portMap := make(nat.PortMap)
	for _, port := range ports {
		containerPort := nat.Port(fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
		portMap[containerPort] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", port.HostPort),
			},
		}
	}
	return portMap
}

func getRestartPolicy(policy string) container.RestartPolicy {
	switch policy {
	case "always":
		return container.RestartPolicy{Name: "always"}
	case "on-failure":
		return container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3}
	case "unless-stopped":
		return container.RestartPolicy{Name: "unless-stopped"}
	default:
		return container.RestartPolicy{Name: "no"}
	}
}

func generateContainerName(serviceName, taskID string) string {
	return fmt.Sprintf("gorchester-%s-%s-%d",
		serviceName,
		taskID,
		time.Now().Unix())
}
