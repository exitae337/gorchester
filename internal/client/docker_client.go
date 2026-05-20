package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/containerd/errdefs"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/exitae337/gorchester/internal/types"
)

// ContainerManager interface realization

const (
	defaultTimeout = 15 * time.Second
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

// Create Container: Create and start container by configuration
func (dc *DockerClient) CreateContainer(ctx context.Context, service *types.ServiceConfig, taskID string, logger *slog.Logger) (string, error) {
	const op = "client.CreateContainer"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	logger.Debug("CreateContainer: checking if image exists", "image", service.Image)
	if exists, err := dc.imageExists(ctx, service.Image); err != nil {
		logger.Error("CreateContainer: failed to check image", "error", err)
		return "", fmt.Errorf("%s: failed to check image locally: %w", op, err)
	} else if !exists {
		logger.Info("CreateContainer: image not found locally, pulling", "image", service.Image)
		if err := dc.PullImage(ctx, service.Image, logger); err != nil {
			logger.Error("CreateContainer: failed to pull image", "error", err)
			return "", fmt.Errorf("%s: failed to download image: %w", op, err)
		}
	}

	logger.Debug("CreateContainer: image ready, creating container")

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

	hostConfig := &container.HostConfig{
		PortBindings:  createPortBindings(service.Ports),
		RestartPolicy: getRestartPolicy(service.RestartPolicy),
		Resources: container.Resources{
			NanoCPUs:   int64(service.Resources.CPUMilliCores * 1_000_000),
			Memory:     service.Resources.MemoryBytes,
			MemorySwap: service.Resources.MemoryBytes,
			CpusetCpus: service.Resources.CPUSet,
		},
		Binds:       service.Volumes,
		NetworkMode: container.NetworkMode(service.NetworkMode),
		DNS:         service.DNS,
		ExtraHosts:  service.ExtraHosts,
	}

	containerName := generateContainerName(service.ServiceName, taskID)
	logger.Debug("CreateContainer: creating container", "name", containerName)

	resp, err := dc.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		containerName,
	)

	if err != nil {
		logger.Error("CreateContainer: failed to create container", "error", err)
		return "", fmt.Errorf("%s: error creating container: %w", op, err)
	}

	logger.Debug("CreateContainer: container created, starting", "container_id", resp.ID[:12])

	if err := dc.StartContainer(ctx, resp.ID); err != nil {
		logger.Error("CreateContainer: failed to start container, cleaning up", "error", err)
		cleanUpCtx, cleanUpCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanUpCancel()
		dc.cli.ContainerRemove(cleanUpCtx, resp.ID, container.RemoveOptions{})
		return "", fmt.Errorf("%s: failed to start container: %w", op, err)
	}

	logger.Info("CreateContainer: container started successfully", "container_id", resp.ID[:12])
	return resp.ID, nil
}

// DisconnectFromNetwork отключает контейнер от всех сетей перед удалением
func (dc *DockerClient) DisconnectFromNetwork(ctx context.Context, containerID string) error {
	const op = "client.DisconnectFromNetwork"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	// Получаем информацию о контейнере
	inspect, err := dc.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		// Контейнер уже удален или не существует — не ошибка
		return nil
	}

	// Отключаем от всех кастомных сетей
	for networkName := range inspect.NetworkSettings.Networks {
		if networkName != "bridge" && networkName != "host" && networkName != "none" {
			if err := dc.cli.NetworkDisconnect(ctx, networkName, containerID, true); err != nil {
				// Логируем, но не считаем критической ошибкой
				continue
			}
		}
	}

	return nil
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

// Stop container
func (dc *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	const op = "client.StopContainer"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	timeout := 10 // Timeout to stop the Container in seconds
	if err := dc.cli.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil {
		return fmt.Errorf("%s: failed to stop container: %s -> error: %w", op, containerID[:12], err)
	}

	return nil
}

// Remove container
func (dc *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	const op = "client.RemoveDocker"
	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	if err := dc.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
		RemoveLinks:   true,
	}); err != nil {
		return fmt.Errorf("%s: error with container %s removing: %w", op, containerID[:12], err)
	}

	return nil
}

// Download Docker image
func (dc *DockerClient) PullImage(ctx context.Context, im string, logger *slog.Logger) error {
	const op = "client.PullImage"

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Big timeout for downloading image: 5 minutes
	defer cancel()

	reader, err := dc.cli.ImagePull(ctx, im, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("%s: failed to pull image %s: %w", op, im, err)
	}
	defer reader.Close()

	// JSON decoder to show stream
	decoder := json.NewDecoder(reader)
	lastLogTime := time.Now()
	logInterval := 3 * time.Second

	for {
		// Message structure for downloading progress
		var msg struct {
			Status   string `json:"status"`
			Progress string `json:"progress"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("%s: failed to parse json messages from PullImage: %w", op, err)
		}

		// Logging every 3 seconds
		if time.Since(lastLogTime) >= logInterval || msg.Status == "Download complete" {
			logger.Info("downloading image", slog.String("status", msg.Status), slog.String("progress", msg.Progress))
			lastLogTime = time.Now()
		}

	}

	return nil
}

// Close закрывает соединение с Docker daemon
func (dc *DockerClient) Close() error {
	const op = "client.Close"

	if dc.cli != nil {
		if err := dc.cli.Close(); err != nil {
			return fmt.Errorf("%s: failed to close docker client: %w", op, err)
		}
	}
	return nil
}

// Get container status func
func (dc *DockerClient) GetConatinerStatus(ctx context.Context, containerID string) (string, error) {
	const op = "client.GetContainerStats"

	ctx, cancel := context.WithTimeout(ctx, dc.timeout)
	defer cancel()

	inspect, err := dc.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return "not_found", err
		}
		return "", fmt.Errorf("%s: failed to inspect container with id: %s, %w", op, containerID[:12], err)
	}

	if !inspect.State.Running {
		return inspect.State.Status, nil
	}

	if inspect.State.Health != nil {
		switch inspect.State.Health.Status {
		case "healthy":
			return "running_healthy", nil
		case "unhealthy":
			return "running_unhealthy", nil
		case "starting":
			return "starting", nil
		}
	}

	return "running", nil
}

// Check Container Health -> by client
func (dc *DockerClient) CheckContainerHealth(ctx context.Context, containerID string, healthOpts *types.HealthCheck) (bool, error) {
	const op = "client.HealthCheck"

	if healthOpts == nil {
		return true, nil
	}

	// Check -> Container exists?
	inspect, err := dc.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, fmt.Errorf("%s: failed to inspect container -> %w", op, err)
	}

	if !inspect.State.Running {
		return false, nil // if not running -> false
	}

	switch healthOpts.Type {
	case "http":
		return dc.checkHealthByHTTP(ctx, containerID, healthOpts)
	case "tcp":
		return dc.checkHealthByTCP(ctx, containerID, healthOpts)
	case "command":
		return dc.checkHealthByCommand(ctx, containerID, healthOpts)
	default:
		return true, nil
	}
}

// GetClient возвращает базового клиента Docker для метрик
func (dc *DockerClient) GetClient() *client.Client {
	return dc.cli
}

func (dc *DockerClient) checkHealthByHTTP(ctx context.Context, containerID string, healthCheck *types.HealthCheck) (bool, error) {
	const op = "client.HttpHealthCheck"
	_, err := dc.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, fmt.Errorf("%s: failed to inspect container -> %w", op, err)
	}

	// Make URL -> Check on host
	url := fmt.Sprintf("http://localhost:%d%s", healthCheck.Port, healthCheck.HTTPPath)

	execConfig := types.ExecConfig{
		Cmd:          []string{"curl", "-f", "-s", "-o", "/dev/null", "-w", "%{http_code}", url},
		AttachStdOut: true,
		AttachStdErr: true,
	}

	// Run command
	exitCode, output, err := dc.execInContainer(ctx, containerID, &execConfig)
	if err != nil {
		return false, fmt.Errorf("%s: failed to exec cmd in Container -> %w", op, err)
	}

	if exitCode != 0 {
		return false, fmt.Errorf("%s: container health check failed -> ContainerID: %s, %s, %d", op, containerID[:12], output, exitCode)
	}

	return true, nil
}

// TCP Health Check func
func (dc *DockerClient) checkHealthByTCP(ctx context.Context, containerID string, healthCheck *types.HealthCheck) (bool, error) {
	const op = "client.checkHealthByTCP"

	timeoutSec := int(healthCheck.Timeout.Seconds())
	if timeoutSec < 1 {
		timeoutSec = 5
	}

	// sh: for Alpine and Debian
	cmd := fmt.Sprintf(
		"sh -c 'timeout %d sh -c \"echo > /dev/tcp/localhost/%d\" 2>/dev/null'",
		timeoutSec,
		healthCheck.Port,
	)

	execConfig := types.ExecConfig{
		Cmd:          []string{"sh", "-c", cmd},
		AttachStdOut: true,
		AttachStdErr: true,
	}

	exitCode, output, err := dc.execInContainer(ctx, containerID, &execConfig)
	if err != nil {
		return false, fmt.Errorf("%s: exec failed due to: %w", op, err)
	}

	if exitCode != 0 {
		return false, fmt.Errorf("TCP Health Check failed: %s, port %d, exit %d, output: %s",
			containerID[:12], healthCheck.Port, exitCode, output)
	}

	return true, nil
}

// Check Health by Command
func (dc *DockerClient) checkHealthByCommand(ctx context.Context, containerID string, healthCheck *types.HealthCheck) (bool, error) {
	const op = "client.checkHealthByCMD"

	execConfig := types.ExecConfig{
		Cmd:          healthCheck.Command,
		AttachStdOut: true,
		AttachStdErr: true,
	}

	exitCode, output, err := dc.execInContainer(ctx, containerID, &execConfig)
	if err != nil {
		return false, fmt.Errorf("%s: failed to check container health by command: %w", op, err)
	}

	if exitCode != 0 {
		return false, fmt.Errorf("CMD Health Check failed: %s, %d, %d, output: %s", containerID, healthCheck.Port, exitCode, output)
	}

	return true, nil
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

// exec in container func -> commands in Container
func (dc *DockerClient) execInContainer(ctx context.Context, containerID string, execConfig *types.ExecConfig) (int, string, error) {
	const op = "client.ExecinContainer"
	exec, err := dc.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          execConfig.Cmd,
		AttachStdout: execConfig.AttachStdOut,
		AttachStderr: execConfig.AttachStdErr,
	})
	if err != nil {
		return -1, "", fmt.Errorf("%s: failed to exec command in container: %w, ContainerID: %s", op, err, containerID)
	}

	// Run exec and get Response
	resp, err := dc.cli.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return -1, "", fmt.Errorf("%s: failed to exec cmd in Container: %w, ContainerID: %s", op, err, containerID)
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return -1, "", fmt.Errorf("%s: failed to read from resp.Reader -> %s", op, err)
	}

	inspectResp, err := dc.cli.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return -1, string(output), fmt.Errorf("%s: failed to get resp from container -> %w", op, err)
	}

	return inspectResp.ExitCode, string(output), nil
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
