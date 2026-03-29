package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/exitae337/gorchester/internal/client"
	"github.com/exitae337/gorchester/internal/types"
	"github.com/google/uuid"
)

// Schdeuler interface

// Scheduler -> SimpleScheduler struct -> Interface
type Scheduler interface {
	// SelectNode -> select node for the Task by settings
	SelectNode(ctx context.Context, task *types.Task, nodes []*types.Node) (string, error)

	// GetNodes -> all applyable Nodes
	GetNodes(ctx context.Context) ([]*types.Node, error)

	// RegisterNode -> New Node in Cluster
	RegisterNode(ctx context.Context, node *types.Node) error

	// UnregisterNode -> Delete Node from Cluster
	UnregisterNode(ctx context.Context, nodeID string) error

	// UpdateNodeStatus -> Update cluster status
	UpdateNodeStatus(ctx context.Context, nodeID string, status types.NodeStatus) error

	// Release Node Resources
	ReleaseNodeResources(ctx context.Context, nodeID string, task *types.Task)
}

// Store interface
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

// Orchestrator settings
type OrchestratorSettings struct {
	ReconcileInterval   time.Duration // reconcile interval
	HealthCheckInterval time.Duration // health check interval
	CleanUpIntarval     time.Duration // clean up interval
	TaskTTL             time.Duration // how long stopped Tasks will be saved (TTL)
}

// DefaultOrchestrator Settings
func DefaultOrchestratorSettings() *OrchestratorSettings {
	return &OrchestratorSettings{
		ReconcileInterval:   30 * time.Second,
		HealthCheckInterval: 15 * time.Second,
		CleanUpIntarval:     30 * time.Minute,
		TaskTTL:             24 * time.Hour,
	}
}

// Orchestrator struct
type Orchestrator struct {
	settings     *OrchestratorSettings
	appConfig    *types.OchestratorConfig
	taskStore    TaskStore
	dockerClient *client.DockerClient
	scheduler    Scheduler

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	logger    *slog.Logger
	isRunning bool
	mu        sync.RWMutex
}

// New Orchestartor -> Constructor
func New(
	appConfig *types.OchestratorConfig,
	taskStore TaskStore,
	dockerClient *client.DockerClient,
	scheduler Scheduler,
	logger *slog.Logger,
) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}

	return &Orchestrator{
		settings:     DefaultOrchestratorSettings(),
		appConfig:    appConfig,
		taskStore:    taskStore,
		dockerClient: dockerClient,
		scheduler:    scheduler,
		logger:       logger.With("component", "orchestrator"),
	}
}

// Start Orchestrator
func (o *Orchestrator) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.isRunning {
		return fmt.Errorf("orchestrator is already running")
	}

	o.ctx, o.cancel = context.WithCancel(context.Background())
	o.isRunning = true

	// Background cycles
	o.wg.Add(3)
	go o.healthCheckLoop()
	go o.reconcileLoop()
	// cleanupLoop()

	// Init services from config
	if err := o.initServices(); err != nil {
		o.logger.Error("failed to init services from config", err)
		o.cancel()
		o.wg.Wait()
		o.isRunning = false
		return fmt.Errorf("services init failed: %w", err)
	}

	o.logger.Info("orchestartor started successfully",
		"cluster", o.appConfig.ClusterName,
		"services", len(o.appConfig.Services))

	return nil
}

// Stop orchestartor -> Error returning? !!!
func (o *Orchestrator) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.isRunning {
		return nil
	}

	o.logger.Info("stopping orchestartor...")
	o.cancel()

	// Waiting for stopping by timeout
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		o.logger.Info("orchestrator stopped gracefully!")
	case <-time.After(30 * time.Second):
		o.logger.Warn("orchestrtor stopping time out")
	}

	o.isRunning = false
	return nil
}

// Start services by init app Config
func (o *Orchestrator) initServices() error {
	ctx := context.Background()

	for _, svc := range o.appConfig.Services {
		o.logger.Info("initializing service",
			"service", svc.ServiceName,
			"replicas", svc.Replicas)

		// How many Tasks we have for this Service now?
		existingTasks, err := o.taskStore.ListByService(ctx, svc.ServiceName)
		if err != nil {
			return fmt.Errorf("failed to list tasks for service %s: %w", svc.ServiceName, err)
		}

		// Make replicas
		for i := len(existingTasks); i < svc.Replicas; i++ {
			if err := o.createServiceTask(ctx, &svc); err != nil {
				o.logger.Error("failed to create task during init",
					"service", svc.ServiceName,
					"error", err)
			}
			time.Sleep(100 * time.Millisecond) // Pause for Docker API
		}
	}
	return nil
}

// Create service Task
func (o *Orchestrator) createServiceTask(ctx context.Context, service *types.ServiceConfig) error {
	taskID := uuid.New().String()

	// Choose Node for Task
	nodes, err := o.scheduler.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// Task For Scheduler -> TEMP (временный)
	tempTask := &types.Task{
		ID:            taskID,
		ServiceName:   service.ServiceName,
		ServiceConfig: service,
	}

	nodeID, err := o.scheduler.SelectNode(ctx, tempTask, nodes)
	if err != nil {
		return fmt.Errorf("failed to select node: %w", err)
	}

	// Make Task
	now := time.Now()
	task := &types.Task{
		ID:            taskID,
		ServiceName:   service.ServiceName,
		Status:        types.TaskStatusPending,
		DesiredState:  types.TaskStatusRunning,
		NodeID:        nodeID,
		CreatedAt:     now,
		UpdatedAt:     now,
		ServiceConfig: service,
		RestartCount:  0,
		PortMapping:   service.Ports,
		Labels: map[string]string{
			"service":    service.ServiceName,
			"created_by": "orchestrator",
		},
	}

	// Save in Store
	if err := o.taskStore.Create(ctx, task); err != nil {
		o.scheduler.ReleaseNodeResources(ctx, nodeID, task)
		return fmt.Errorf("failed to save task: %w", err)
	}

	o.logger.Info("task created and saved",
		"task_id", taskID,
		"service", service.ServiceName,
		"node", nodeID)

	// Do Task (асинхронно)
	go o.executeTask(task)

	return nil
}

// executeTask do Task -> make docker container
func (o *Orchestrator) executeTask(task *types.Task) {
	// Используем отдельный контекст для этой операции, но привязываем к общему o.ctx (при остановке оркестратора, все
	// опреации дожны прерываться) -> ctx.cancel() -> stopping all containers
	ctx, cancel := context.WithCancel(o.ctx)
	defer cancel()

	taskLogger := o.logger.With(
		"task_id", task.ID,
		"service", task.ServiceName,
	)

	taskLogger.Info("executing task")

	// 1. Update status on Starting
	task.Status = types.TaskStatusStarting
	if err := o.taskStore.Update(ctx, task); err != nil {
		taskLogger.Error("failed to update task status to starting", "error", err)
		// Not critical
	}

	// 2. Make Container by DockerClient
	containerID, err := o.dockerClient.CreateContainer(
		ctx,
		task.ServiceConfig,
		task.ID,
		taskLogger, // <- Logger with Task context
	)

	if err != nil {
		taskLogger.Error("failed to create/start container", "error", err)
		// if error -> status failed
		task.Status = types.TaskStatusFailed
		task.Error = err.Error()
		now := time.Now()
		task.FinishedAt = &now

		if updaterErr := o.taskStore.Update(ctx, task); updaterErr != nil {
			taskLogger.Error("failed to update task status to failed", "error", updaterErr)
		}

		o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
		return
	}

	// 3. ALL OK -> Update Task
	task.ContainerID = containerID
	task.Status = types.TaskStatusRunning
	now := time.Now()
	task.StartedAt = &now

	if err := o.taskStore.Update(ctx, task); err != nil {
		taskLogger.Error("failed to update task status to running", "error", err)
		// CRITICAL -> try to Stop container
		o.dockerClient.StopContainer(ctx, containerID)
		o.dockerClient.RemoveContainer(ctx, containerID)
		o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
		return
	}

	taskLogger.Info("container started successfully", "container_id", containerID[:12])
}

// reconcileLoop -> start reconcile
func (o *Orchestrator) reconcileLoop() {
	defer o.wg.Done()
	ticker := time.NewTicker(o.settings.ReconcileInterval)
	defer ticker.Stop()

	o.logger.Info("reconcile loop started", "interval", o.settings.ReconcileInterval)

	for {
		select {
		case <-o.ctx.Done():
			o.logger.Info("reconcile loop stopped")
			return
		case <-ticker.C:
			o.reconcile()
		}
	}
}

// reconcile -> check and fix cluster status
func (o *Orchestrator) reconcile() {
	ctx := context.Background()
	o.logger.Debug("starting reconciliation")

	// 1. Get all Tasks that we have
	tasks, err := o.taskStore.List(ctx)
	if err != nil {
		o.logger.Error("failed to list tasks", "error", err)
		return
	}

	// 2. Group Tasks by service
	tasksByService := make(map[string][]*types.Task)
	for _, task := range tasks {
		tasksByService[task.ServiceName] = append(tasksByService[task.ServiceName], task)
	}

	// 3. For each service in config
	for _, svc := range o.appConfig.Services {
		serviceTasks := tasksByService[svc.ServiceName]
		if serviceTasks == nil {
			serviceTasks = []*types.Task{}
		}

		// Count Tasks in different statuses
		var running, pending, failed, stopped int
		for _, t := range serviceTasks {
			switch t.Status {
			case types.TaskStatusRunning:
				running++
			case types.TaskStatusPending:
				pending++
			case types.TaskStatusFailed:
				failed++
			case types.TaskStatusStopped:
				stopped++
			}
		}

		// 4. Make Sure, which Tasks need restarting
		for _, task := range serviceTasks {
			if task.NeedsRestart() {
				o.logger.Info("task needs restart",
					"task_id", task.ID,
					"status", task.Status,
					"desired", task.DesiredState)

				// restart_counter++
				o.taskStore.IncrementRestartCounter(ctx, task.ID)

				// Release resources -> old Task
				if task.NodeID != "" {
					o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
				}

				// Make new Task (old Tast to delete) OR make new Replica TODO
				if err := o.createServiceTask(ctx, &svc); err != nil {
					o.logger.Error("failed to create replacement task", "error", err)
				}
			}
		}

		// 5. Check replicas count -> scale policy
		desiredReplicas := svc.Replicas // base counter
		if svc.ScalePolicy.MinReplicas > 0 {
			// if min_replicas -> use it (min border)
			desiredReplicas = svc.ScalePolicy.MinReplicas
		}

		// AUTOSCALING !!! TODO !!!

		currentReplicas := running + pending // Pending -> Running

		if currentReplicas < desiredReplicas {
			missing := desiredReplicas - currentReplicas
			o.logger.Info("scaling up",
				"service", svc.ServiceName,
				"current", currentReplicas,
				"desired", desiredReplicas,
				"missing", missing)

			for i := 0; i < missing; i++ {
				if err := o.createServiceTask(ctx, &svc); err != nil {
					o.logger.Error("failed to scale up", "error", err)
					break
				}
				time.Sleep(100 * time.Millisecond) // for Docker API :)
			}
		} else if currentReplicas > desiredReplicas && svc.ScalePolicy.MaxReplicas > 0 && currentReplicas > svc.ScalePolicy.MaxReplicas {
			// If current > max_replicas -> low it down
			excess := currentReplicas - svc.ScalePolicy.MaxReplicas
			o.logger.Info("scaling down (exceeds max)",
				"service", svc.ServiceName,
				"current", currentReplicas,
				"max", svc.ScalePolicy.MaxReplicas,
				"excess", excess)
			// TODO: реализовать остановку лишних задач
			o.scaleDown(ctx, &svc, tasks, excess)
		}
	}

	o.logger.Debug("reconciliation completed")
}

// scaleDown -> stops excees tasks
func (o *Orchestrator) scaleDown(ctx context.Context, svc *types.ServiceConfig, tasks []*types.Task, excess int) {
	for i := 0; i < excess && i < len(tasks); i++ {
		task := tasks[len(tasks)-1-i]
		if task.Status != types.TaskStatusRunning {
			continue
		}

		o.logger.Info("stopping task for scale down",
			"task_id", task.ID,
			"service", task.ServiceName)

		// Update Desired state
		task.DesiredState = types.TaskStatusStopped
		o.taskStore.Update(ctx, task)

		// Stop and Delete container
		if task.ContainerID != "" {
			if err := o.dockerClient.StopContainer(ctx, task.ContainerID); err != nil {
				o.logger.Warn("failed to stop container during scale down",
					"task_id", task.ID,
					"error", err)
			}
			if err := o.dockerClient.RemoveContainer(ctx, task.ContainerID); err != nil {
				o.logger.Warn("failed to remove container during scale down",
					"task_id", task.ID,
					"error", err)
			}
		}

		// Release resources
		if task.NodeID != "" {
			o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
		}

		// Task -> change status to stopped
		task.Status = types.TaskStatusStopped
		now := time.Now()
		task.FinishedAt = &now
		o.taskStore.Update(ctx, task)
	}
}

// healthCheckLoop -> Health check loop
func (o *Orchestrator) healthCheckLoop() {
	defer o.wg.Done()
	ticker := time.NewTicker(o.settings.HealthCheckInterval)
	defer ticker.Stop()

	o.logger.Info("health check loop started", "interval", o.settings.HealthCheckInterval)

	for {
		select {
		case <-o.ctx.Done():
			o.logger.Info("health check loop stopped")
			return
		case <-ticker.C:
			o.checkHealth()
		}
	}
}

// checkHealth -> check health
// !!! NO SUCH METHOD IN DOCKER CLIENT !!!
func (o *Orchestrator) checkHealth() {
	ctx := context.Background()

	tasks, err := o.taskStore.ListByStatus(ctx, types.TaskStatusRunning)
	if err != nil {
		o.logger.Error("failed to list running tasks", "error", err)
		return
	}

	for _, task := range tasks {
		if task.ServiceConfig == nil {
			continue
		}
		healthy := true

		if !healthy {
			o.logger.Warn("container unhealthy",
				"task_id", task.ID,
				"service", task.ServiceName)

			// Task status = failed
			task.Status = types.TaskStatusFailed
			now := time.Now()
			task.FinishedAt = &now
			o.taskStore.Update(ctx, task)

			// Release resources
			if task.NodeID != "" {
				o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
			}
		}
	}
	// TODO -> By Docker Client Method (internal/client)
	// healthy, err := o.dockerClient.CheckContainerHealth(ctx, task.ContainerID, task.ServiceConfig.HealthCheck)
	// if err != nil {
	//     o.logger.Error("health check failed", "task_id", task.ID, "error", err)
	//     task.UpdateTask(TaskStatusFailed)
	//     o.taskStore.Update(ctx, task)
	// }

	o.logger.Debug("health check completed", "checked_count", len(tasks))
}

// cleanUpLoop
func (o *Orchestrator) cleanUpLoop() {
	defer o.wg.Done()
	ticker := time.NewTicker(o.settings.CleanUpIntarval)
	defer ticker.Stop()

	o.logger.Info("cleaup loop started", "interval", o.settings.CleanUpIntarval)

	for {
		select {
		case <-o.ctx.Done():
			o.logger.Info("cleanup ended successfully")
			return
		case <-ticker.C:
			o.cleanup()
		}
	}
}

// cleanup
func (o *Orchestrator) cleanup() {
	ctx := context.Background()

	// All terminated tasks
	terminatedTasks, err := o.taskStore.ListByStatus(ctx, types.TaskStatusDead)
	if err != nil {
		o.logger.Error("failed to get dead tasks from taskStore", "error", err)
		return
	}

	// Delete tasks older than TaskTTL
	cutoff := time.Now().Add(-o.settings.TaskTTL)
	for _, task := range terminatedTasks {
		if task.FinishedAt != nil && task.FinishedAt.Before(cutoff) {
			o.logger.Info("cleaning up old task",
				"task_id", task.ID,
				"finished_at", task.FinishedAt)

			// release old resources (if not done yet)
			if task.NodeID != "" {
				o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
			}

			if err := o.taskStore.Delete(ctx, task.ID); err != nil {
				o.logger.Error("failed to delete old task",
					"task_id", task.ID,
					"error", err)
			}
		}
	}

}

// GetTask -> Get Task by ID (API Method)
func (o *Orchestrator) GetTask(ctx context.Context, id string) (*types.Task, error) {
	return o.taskStore.Get(ctx, id)
}

// ListTasks -> Get All Tasks (API Method)
func (o *Orchestrator) ListTasks(ctx context.Context) ([]*types.Task, error) {
	return o.taskStore.List(ctx)
}

// DeleteTask - delete Task (API Method)
func (o *Orchestrator) DeleteTask(ctx context.Context, id string) error {
	task, err := o.taskStore.Get(ctx, id)
	if err != nil {
		return err
	}

	// If container is already started -> delete it
	if task.ContainerID != "" {
		if err := o.dockerClient.StopContainer(ctx, task.ContainerID); err != nil {
			o.logger.Warn("failed to stop container during task deletion",
				"task_id", id,
				"container", task.ContainerID[:12],
				"error", err)
			// Delete Task from TaskStore
		}
		if err := o.dockerClient.RemoveContainer(ctx, task.ContainerID); err != nil {
			o.logger.Warn("failed to remove container during task deletion",
				"task_id", id,
				"container", task.ContainerID[:12],
				"error", err)
		}
	}

	if task.NodeID != "" {
		o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
	}

	return o.taskStore.Delete(ctx, id)
}
