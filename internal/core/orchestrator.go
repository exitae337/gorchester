package core

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/exitae337/gorchester/internal/client"
	"github.com/exitae337/gorchester/internal/metrics"
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
	ReleaseNodeResources(ctx context.Context, nodeID string, task *types.Task) error
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

	metricsCollector *metrics.MetricsCollector
	metricsStore     *metrics.MetricsStore
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

	metricsCollector := metrics.NewMetricscollector(dockerClient.GetClient())

	return &Orchestrator{
		settings:         DefaultOrchestratorSettings(),
		appConfig:        appConfig,
		taskStore:        taskStore,
		dockerClient:     dockerClient,
		scheduler:        scheduler,
		logger:           logger.With("component", "orchestrator"),
		metricsCollector: metricsCollector,
		metricsStore:     metrics.NewMetricsStore(1000),
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
	go o.cleanUpLoop()

	// Init services from config
	if err := o.initServices(); err != nil {
		o.logger.Error("failed to init services from config", slog.Any("error", err))
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

	o.logger.Info("stopping orchestrator...")
	o.cancel()

	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		o.logger.Info("orchestrator stopped gracefully")
	case <-time.After(30 * time.Second):
		o.logger.Warn("orchestrator stopping timed out after 30s")
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

// Метод для сбора метрик
func (o *Orchestrator) collectMetrics(ctx context.Context) {
	tasks, err := o.taskStore.ListByStatus(ctx, types.TaskStatusRunning)
	if err != nil {
		o.logger.Error("failed to list running tasks for metrics", "error", err)
		return
	}

	o.logger.Debug("collectMetrics: found running tasks", "count", len(tasks))

	if len(tasks) == 0 {
		o.logger.Warn("collectMetrics: no running tasks found - metrics collection skipped")
		return
	}

	serviceMetrics := make(map[string]*types.ServiceMetrics)
	collectedCount := 0
	failedCount := 0

	for _, task := range tasks {
		if task.ContainerID == "" {
			o.logger.Debug("collectMetrics: task has no ContainerID", "task_id", task.ID)
			continue
		}

		o.logger.Debug("collectMetrics: collecting from container",
			"task_id", task.ID,
			"container_id", task.ContainerID[:12])

		metric, err := o.metricsCollector.CollectContainerMetrics(ctx, task.ContainerID)
		if err != nil {
			o.logger.Error("collectMetrics: failed to collect",
				"task_id", task.ID,
				"container_id", task.ContainerID[:12],
				"error", err)
			failedCount++
			continue
		}

		metric.TaskID = task.ID
		metric.ServiceName = task.ServiceName
		o.metricsStore.StoreMetrics(metric)
		collectedCount++

		// Агрегация по сервисам (твой существующий код)
		if _, exists := serviceMetrics[task.ServiceName]; !exists {
			serviceMetrics[task.ServiceName] = &types.ServiceMetrics{
				ServiceName: task.ServiceName,
				Timestamp:   time.Now(),
			}
		}
		sm := serviceMetrics[task.ServiceName]
		sm.AvgCPUPercent += metric.CPUPercent
		sm.AvgMemoryPercent += metric.MemoryPercent
		sm.TotalContainers++
	}

	o.logger.Info("collectMetrics completed",
		"collected", collectedCount,
		"failed", failedCount,
		"total_tasks", len(tasks))

	// Усреднение (твой существующий код)
	for _, sm := range serviceMetrics {
		if sm.TotalContainers > 0 {
			sm.AvgCPUPercent /= float64(sm.TotalContainers)
			sm.AvgMemoryPercent /= float64(sm.TotalContainers)
		}
		o.logger.Debug("service metrics",
			"service", sm.ServiceName,
			"avg_cpu", fmt.Sprintf("%.2f%%", sm.AvgCPUPercent),
			"avg_mem", fmt.Sprintf("%.2f%%", sm.AvgMemoryPercent),
			"containers", sm.TotalContainers)
	}
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
	ctx, cancel := context.WithCancel(o.ctx)
	defer cancel()

	taskLogger := o.logger.With(
		"task_id", task.ID,
		"service", task.ServiceName,
	)

	taskLogger.Info("executeTask: starting execution")

	// 1. Update status to Starting
	task.Status = types.TaskStatusStarting
	if err := o.taskStore.Update(ctx, task); err != nil {
		taskLogger.Error("executeTask: failed to update status to starting", "error", err)
	}

	// 2. Check if ServiceConfig is nil
	if task.ServiceConfig == nil {
		taskLogger.Error("executeTask: ServiceConfig is nil - cannot create container")
		task.Status = types.TaskStatusFailed
		task.Error = "ServiceConfig is nil"
		now := time.Now()
		task.FinishedAt = &now
		o.taskStore.Update(ctx, task)
		return
	}

	taskLogger.Debug("executeTask: creating container",
		"image", task.ServiceConfig.Image,
		"node", task.NodeID)

	// 3. Create Container
	containerID, err := o.dockerClient.CreateContainer(
		ctx,
		task.ServiceConfig,
		task.ID,
		taskLogger,
	)

	if err != nil {
		taskLogger.Error("executeTask: failed to create/start container", "error", err)
		task.Status = types.TaskStatusFailed
		task.Error = err.Error()
		now := time.Now()
		task.FinishedAt = &now

		if updateErr := o.taskStore.Update(ctx, task); updateErr != nil {
			taskLogger.Error("executeTask: failed to update task status to failed", "error", updateErr)
		}

		o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
		return
	}

	// 4. Success - update task
	task.ContainerID = containerID
	task.Status = types.TaskStatusRunning
	now := time.Now()
	task.StartedAt = &now

	if err := o.taskStore.Update(ctx, task); err != nil {
		taskLogger.Error("executeTask: failed to update status to running - rolling back", "error", err)
		// Rollback: stop and remove container
		o.dockerClient.StopContainer(ctx, containerID)
		o.dockerClient.RemoveContainer(ctx, containerID)
		o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
		return
	}

	taskLogger.Info("executeTask: container started successfully", "container_id", containerID[:12])
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
	ctx := o.ctx // Use orchestrator context
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

	// 3. Collect metrics for all running containers
	o.collectMetrics(ctx)

	// 4. For each service in config
	for i := range o.appConfig.Services {
		svc := &o.appConfig.Services[i] // Работаем с указателем для возможности изменения

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

		// 5. Make Sure, which Tasks need restarting
		for _, task := range serviceTasks {
			if task.NeedsRestart() {
				o.logger.Info("task needs restart",
					"task_id", task.ID,
					"service", task.ServiceName,
					"status", task.Status,
					"desired", task.DesiredState)

				// restart_counter++
				if err := o.taskStore.IncrementRestartCounter(ctx, task.ID); err != nil {
					o.logger.Error("failed to increment restart counter",
						"task_id", task.ID,
						"error", err)
				}

				// Release resources -> old Task
				if task.NodeID != "" {
					if err := o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task); err != nil {
						o.logger.Error("failed to release resources",
							"task_id", task.ID,
							"node", task.NodeID,
							"error", err)
					}
				}

				// Mark old task as stopped
				task.Status = types.TaskStatusStopped
				now := time.Now()
				task.FinishedAt = &now
				task.DesiredState = types.TaskStatusStopped
				if err := o.taskStore.Update(ctx, task); err != nil {
					o.logger.Error("failed to update stopped task",
						"task_id", task.ID,
						"error", err)
				}

				// Stop and remove container if exists
				if task.ContainerID != "" {
					if err := o.dockerClient.StopContainer(ctx, task.ContainerID); err != nil {
						o.logger.Warn("failed to stop container for restart",
							"task_id", task.ID,
							"container", task.ContainerID[:12],
							"error", err)
					}
					if err := o.dockerClient.RemoveContainer(ctx, task.ContainerID); err != nil {
						o.logger.Warn("failed to remove container for restart",
							"task_id", task.ID,
							"container", task.ContainerID[:12],
							"error", err)
					}
				}

				// Create replacement task
				o.logger.Info("creating replacement task",
					"service", svc.ServiceName,
					"old_task", task.ID)

				if err := o.createServiceTask(ctx, svc); err != nil {
					o.logger.Error("failed to create replacement task",
						"service", svc.ServiceName,
						"error", err)
				}
			}
		}

		// 6. Calculate desired replicas
		currentReplicas := running + pending
		desiredReplicas := o.calculateDesiredReplicas(svc, currentReplicas)

		// 7. Apply scaling if needed
		if currentReplicas < desiredReplicas {
			// Scale UP
			missing := desiredReplicas - currentReplicas
			o.logger.Info("scaling up",
				"service", svc.ServiceName,
				"current", currentReplicas,
				"desired", desiredReplicas,
				"missing", missing)

			for i := 0; i < missing; i++ {
				if err := o.createServiceTask(ctx, svc); err != nil {
					o.logger.Error("failed to scale up",
						"service", svc.ServiceName,
						"attempt", i+1,
						"error", err)
					break
				}
				time.Sleep(100 * time.Millisecond) // Rate limiting for Docker API
			}
		} else if currentReplicas > desiredReplicas {
			// Scale DOWN
			excess := currentReplicas - desiredReplicas
			o.logger.Info("scaling down",
				"service", svc.ServiceName,
				"current", currentReplicas,
				"desired", desiredReplicas,
				"excess", excess)

			// Pass only this service's tasks for scale down
			o.scaleDownService(ctx, svc, serviceTasks, excess)
		}

		// 8. Apply predictive scaling if enabled
		if svc.ScalePolicy.PredictiveScaling != nil &&
			svc.ScalePolicy.PredictiveScaling.Enabled {
			o.applyPredictiveScaling(ctx, svc)
		}

		// Log service status after reconciliation
		o.logger.Debug("service reconciled",
			"service", svc.ServiceName,
			"replicas", svc.Replicas,
			"running", running,
			"pending", pending,
			"failed", failed,
			"stopped", stopped)
	}

	// 9. Cleanup orphaned tasks (not in config anymore)
	o.cleanupOrphanedTasks(ctx, tasks, tasksByService)

	o.logger.Debug("reconciliation completed")
}

// calculateDesiredReplicas определяет желаемое количество реплик с учётом всех политик
func (o *Orchestrator) calculateDesiredReplicas(service *types.ServiceConfig, currentReplicas int) int {
	desired := service.Replicas

	// Учитываем текущее состояние для более умного масштабирования
	o.logger.Debug("calculating desired replicas",
		"service", service.ServiceName,
		"base_desired", desired,
		"current_replicas", currentReplicas,
		"min_replicas", service.ScalePolicy.MinReplicas,
		"max_replicas", service.ScalePolicy.MaxReplicas)

	// Применяем min/max границы
	if desired < service.ScalePolicy.MinReplicas {
		o.logger.Debug("desired below min, adjusting up",
			"service", service.ServiceName,
			"from", desired,
			"to", service.ScalePolicy.MinReplicas)
		desired = service.ScalePolicy.MinReplicas
	}

	if service.ScalePolicy.MaxReplicas > 0 && desired > service.ScalePolicy.MaxReplicas {
		o.logger.Debug("desired above max, adjusting down",
			"service", service.ServiceName,
			"from", desired,
			"to", service.ScalePolicy.MaxReplicas)
		desired = service.ScalePolicy.MaxReplicas
	}

	// Анализируем разницу с текущим состоянием
	diff := desired - currentReplicas
	if diff > 0 {
		o.logger.Debug("need to scale up",
			"service", service.ServiceName,
			"missing_replicas", diff)
	} else if diff < 0 {
		o.logger.Debug("need to scale down",
			"service", service.ServiceName,
			"excess_replicas", -diff)
	} else {
		o.logger.Debug("replicas count is optimal",
			"service", service.ServiceName)
	}

	return desired
}

// scaleDownService останавливает лишние реплики конкретного сервиса
func (o *Orchestrator) scaleDownService(ctx context.Context, service *types.ServiceConfig, tasks []*types.Task, excess int) {
	// Находим running задачи для остановки
	runningTasks := make([]*types.Task, 0)
	for _, task := range tasks {
		if task.Status == types.TaskStatusRunning {
			runningTasks = append(runningTasks, task)
		}
	}

	if len(runningTasks) == 0 {
		o.logger.Warn("no running tasks to scale down",
			"service", service.ServiceName,
			"excess", excess)
		return
	}

	// Сортируем задачи с учётом preferences сервиса
	o.sortTasksForScaleDown(runningTasks, service)

	// Останавливаем нужное количество задач
	stopped := 0
	for i := 0; i < len(runningTasks) && stopped < excess; i++ {
		task := runningTasks[i]

		// Проверяем, не нарушит ли остановка этой задачи ограничения сервиса
		if !o.canStopTask(ctx, task, service, tasks, stopped) {
			o.logger.Debug("skipping task stop due to service constraints",
				"task_id", task.ID,
				"service", service.ServiceName)
			continue
		}

		o.logger.Info("stopping task for scale down",
			"task_id", task.ID,
			"service", task.ServiceName,
			"node", task.NodeID,
			"stopped_count", stopped+1,
			"total_to_stop", excess,
			"service_type", service.ServiceType)

		// Update desired state
		task.DesiredState = types.TaskStatusStopped
		if err := o.taskStore.Update(ctx, task); err != nil {
			o.logger.Error("failed to update task desired state",
				"task_id", task.ID,
				"error", err)
			continue
		}

		// Stop container
		if task.ContainerID != "" {
			if err := o.dockerClient.StopContainer(ctx, task.ContainerID); err != nil {
				o.logger.Warn("failed to stop container during scale down",
					"task_id", task.ID,
					"container", task.ContainerID[:12],
					"error", err)
			}
			if err := o.dockerClient.RemoveContainer(ctx, task.ContainerID); err != nil {
				o.logger.Warn("failed to remove container during scale down",
					"task_id", task.ID,
					"container", task.ContainerID[:12],
					"error", err)
			}
		}

		// Release resources
		if task.NodeID != "" {
			if err := o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task); err != nil {
				o.logger.Error("failed to release resources",
					"task_id", task.ID,
					"node", task.NodeID,
					"error", err)
			}
		}

		// Update task status
		task.Status = types.TaskStatusStopped
		now := time.Now()
		task.FinishedAt = &now
		if err := o.taskStore.Update(ctx, task); err != nil {
			o.logger.Error("failed to update task status to stopped",
				"task_id", task.ID,
				"error", err)
		}

		stopped++
	}

	// Логируем результат масштабирования
	if stopped < excess {
		o.logger.Warn("could not scale down all excess tasks",
			"service", service.ServiceName,
			"desired_stop", excess,
			"actually_stopped", stopped,
			"reason", "service constraints prevented further scale down")
	}
}

// sortTasksForScaleDown сортирует задачи для остановки с учётом типа сервиса
func (o *Orchestrator) sortTasksForScaleDown(tasks []*types.Task, service *types.ServiceConfig) {
	// Базовая сортировка: сначала задачи на самых загруженных нодах
	sort.Slice(tasks, func(i, j int) bool {
		// Для stateful сервисов - останавливаем задачи на менее предпочтительных нодах
		if service.ServiceType == types.ServiceTypeStateful {
			return o.isNodePreferredForStateful(o.ctx, tasks[i], service) >
				o.isNodePreferredForStateful(o.ctx, tasks[j], service)
		}

		// Для stateless - останавливаем задачи на самых загруженных нодах
		if service.ServiceType == types.ServiceTypeStateless {
			return tasks[i].NodeID > tasks[j].NodeID // простая эвристика
		}

		// По умолчанию - останавливаем более старые задачи
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}

// canStopTask исправленная версия (принимает контекст и использует allTasks правильно)
func (o *Orchestrator) canStopTask(ctx context.Context, task *types.Task, service *types.ServiceConfig, allTasks []*types.Task, alreadyStopped int) bool {
	// Для daemon сервисов - нельзя останавливать последнюю задачу на ноде
	if service.ServiceType == types.ServiceTypeDaemon {
		tasksOnSameNode := 0
		for _, t := range allTasks {
			if t.NodeID == task.NodeID && t.Status == types.TaskStatusRunning {
				tasksOnSameNode++
			}
		}

		// Учитываем уже остановленные задачи в этом цикле масштабирования
		effectiveTasksOnNode := tasksOnSameNode - alreadyStopped

		if effectiveTasksOnNode <= 1 {
			o.logger.Debug("cannot stop last daemon task on node",
				"task_id", task.ID,
				"node", task.NodeID,
				"effective_tasks_on_node", effectiveTasksOnNode)
			return false
		}
	}

	// Для stateful сервисов с anti-affinity - проверяем зоны
	if service.ServiceType == types.ServiceTypeStateful &&
		service.SchedulingConstraints != nil {
		taskZone := o.getNodeZone(ctx, task.NodeID)

		tasksInSameZone := 0
		for _, t := range allTasks {
			if t.ID != task.ID &&
				t.Status == types.TaskStatusRunning &&
				o.getNodeZone(ctx, t.NodeID) == taskZone {
				tasksInSameZone++
			}
		}

		// Учитываем сколько задач из этой зоны уже помечено на остановку
		effectiveTasksInZone := tasksInSameZone - alreadyStopped

		if effectiveTasksInZone <= 0 {
			o.logger.Debug("cannot stop task - zone would have no replicas",
				"task_id", task.ID,
				"zone", taskZone,
				"remaining_tasks", effectiveTasksInZone)
			return false
		}
	}

	// Проверяем общее количество реплик с учётом уже остановленных
	runningCount := 0
	for _, t := range allTasks {
		if t.Status == types.TaskStatusRunning {
			runningCount++
		}
	}

	remainingAfterStop := runningCount - alreadyStopped - 1

	if remainingAfterStop < service.ScalePolicy.MinReplicas {
		o.logger.Debug("cannot stop task - would violate min replicas",
			"task_id", task.ID,
			"remaining_after_stop", remainingAfterStop,
			"min_replicas", service.ScalePolicy.MinReplicas)
		return false
	}

	return true
}

// isNodePreferredForStateful с контекстом
func (o *Orchestrator) isNodePreferredForStateful(ctx context.Context, task *types.Task, service *types.ServiceConfig) int {
	if service.SchedulingConstraints == nil {
		return 0
	}

	taskZone := o.getNodeZone(ctx, task.NodeID)

	for _, rule := range service.SchedulingConstraints.Affinity {
		if rule.Type == "zone" {
			for _, preferredZone := range rule.Values {
				if taskZone == preferredZone {
					return 1 // Предпочтительная зона
				}
			}
		}
	}
	return 0 // Не предпочтительная зона
}

// getNodeZone с контекстом
func (o *Orchestrator) getNodeZone(ctx context.Context, nodeID string) string {
	nodes, err := o.scheduler.GetNodes(ctx)
	if err != nil {
		return "unknown"
	}

	for _, node := range nodes {
		if node.ID == nodeID {
			if zone, exists := node.Labels["zone"]; exists {
				return zone
			}
		}
	}
	return "unknown"
}

// cleanupOrphanedTasks удаляет задачи, сервисы которых больше не в конфигурации
func (o *Orchestrator) cleanupOrphanedTasks(ctx context.Context, allTasks []*types.Task, tasksByService map[string][]*types.Task) {
	// Создаём множество активных сервисов
	activeServices := make(map[string]bool)
	for _, svc := range o.appConfig.Services {
		activeServices[svc.ServiceName] = true
	}

	// Находим orphaned задачи, используя переданный tasksByService
	for serviceName, tasks := range tasksByService {
		if !activeServices[serviceName] {
			o.logger.Info("found orphaned service tasks - cleaning up",
				"service", serviceName,
				"task_count", len(tasks))

			// Используем allTasks для получения полной информации о задачах
			orphanedTasks := make([]*types.Task, 0)
			for _, task := range allTasks {
				if task.ServiceName == serviceName {
					orphanedTasks = append(orphanedTasks, task)
				}
			}

			// Останавливаем все задачи этого сервиса
			o.scaleDownService(ctx, &types.ServiceConfig{
				ServiceName: serviceName,
			}, orphanedTasks, len(orphanedTasks))

			// Логируем сколько задач было очищено
			o.logger.Info("orphaned service cleanup completed",
				"service", serviceName,
				"tasks_cleaned", len(orphanedTasks))
		}
	}
}

// Предиктивное масштабирование
func (o *Orchestrator) applyPredictiveScaling(ctx context.Context, service *types.ServiceConfig) {
	config := service.ScalePolicy.PredictiveScaling
	if config == nil || !config.Enabled {
		return
	}

	// Получаем историю метрик
	metricsHistory := o.metricsStore.GetMetricsHistory(service.ServiceName)
	if len(metricsHistory) < 5 {
		o.logger.Debug("insufficient metrics history for prediction",
			"service", service.ServiceName,
			"data_points", len(metricsHistory))
		return // нужно минимум 5 точек для предсказания
	}

	// Текущие метрики (за последнюю минуту)
	currentMetrics, err := o.metricsStore.GetServiceMetrics(service.ServiceName, 1*time.Minute)
	if err != nil {
		o.logger.Debug("failed to get current metrics",
			"service", service.ServiceName,
			"error", err)
		return
	}

	// Предсказываем через линейную регрессию
	predictedCPU := o.predictCPU(metricsHistory, config.PredictionWindow)
	predictedMem := o.predictMemory(metricsHistory, config.PredictionWindow)

	// Логируем анализ
	o.logger.Debug("predictive scaling analysis",
		"service", service.ServiceName,
		"current_cpu", fmt.Sprintf("%.2f%%", currentMetrics.AvgCPUPercent),
		"predicted_cpu", fmt.Sprintf("%.2f%%", predictedCPU),
		"current_mem", fmt.Sprintf("%.2f%%", currentMetrics.AvgMemoryPercent),
		"predicted_mem", fmt.Sprintf("%.2f%%", predictedMem),
		"cpu_threshold", fmt.Sprintf("%.2f%%", config.CPUThreshold),
		"mem_threshold", fmt.Sprintf("%.2f%%", config.MemoryThreshold),
		"replicas", service.Replicas,
		"min_replicas", service.ScalePolicy.MinReplicas,
		"max_replicas", service.ScalePolicy.MaxReplicas)

	// Проверяем необходимость масштабирования ВВЕРХ
	needScaleUp := o.checkScaleUpConditions(currentMetrics, predictedCPU, predictedMem, config, service)

	// Проверяем необходимость масштабирования ВНИЗ
	needScaleDown := o.checkScaleDownConditions(currentMetrics, predictedCPU, predictedMem, config, service)

	// Применяем масштабирование
	if needScaleUp {
		o.performScaleUp(ctx, service, currentMetrics, predictedCPU, predictedMem)
	} else if needScaleDown {
		o.performScaleDown(ctx, service, currentMetrics, predictedCPU, predictedMem)
	}
}

// checkScaleUpConditions проверяет условия для масштабирования вверх
func (o *Orchestrator) checkScaleUpConditions(
	currentMetrics *types.ServiceMetrics,
	predictedCPU, predictedMem float64,
	config *types.PredictiveScalingConfig,
	service *types.ServiceConfig,
) bool {
	// Уже на максимуме
	if service.Replicas >= service.ScalePolicy.MaxReplicas {
		return false
	}

	// Критические условия - немедленное масштабирование
	if currentMetrics.AvgCPUPercent > config.CPUThreshold ||
		currentMetrics.AvgMemoryPercent > config.MemoryThreshold {
		o.logger.Debug("current load exceeds threshold - immediate scale up",
			"service", service.ServiceName,
			"current_cpu", fmt.Sprintf("%.2f%%", currentMetrics.AvgCPUPercent),
			"current_mem", fmt.Sprintf("%.2f%%", currentMetrics.AvgMemoryPercent))
		return true
	}

	// Предиктивные условия - предсказываем рост
	if predictedCPU > config.CPUThreshold || predictedMem > config.MemoryThreshold {
		o.logger.Debug("predicted load exceeds threshold - predictive scale up",
			"service", service.ServiceName,
			"predicted_cpu", fmt.Sprintf("%.2f%%", predictedCPU),
			"predicted_mem", fmt.Sprintf("%.2f%%", predictedMem))
		return true
	}

	// Комбинированное условие (CPU + Memory)
	combinedLoad := (currentMetrics.AvgCPUPercent/config.CPUThreshold +
		currentMetrics.AvgMemoryPercent/config.MemoryThreshold) / 2.0

	if combinedLoad > 0.9 { // 90% комбинированной нагрузки
		o.logger.Debug("combined load near threshold - scale up",
			"service", service.ServiceName,
			"combined_load", fmt.Sprintf("%.2f", combinedLoad))
		return true
	}

	return false
}

// checkScaleDownConditions проверяет условия для масштабирования вниз
func (o *Orchestrator) checkScaleDownConditions(
	currentMetrics *types.ServiceMetrics,
	predictedCPU, predictedMem float64,
	config *types.PredictiveScalingConfig,
	service *types.ServiceConfig,
) bool {
	// Уже на минимуме
	if service.Replicas <= service.ScalePolicy.MinReplicas {
		return false
	}

	// Все метрики должны быть ниже порога снижения
	scaleDownThreshold := 0.5 // 50% от целевого порога

	cpuBelowThreshold := currentMetrics.AvgCPUPercent < config.CPUThreshold*scaleDownThreshold &&
		predictedCPU < config.CPUThreshold*scaleDownThreshold

	memBelowThreshold := currentMetrics.AvgMemoryPercent < config.MemoryThreshold*scaleDownThreshold &&
		predictedMem < config.MemoryThreshold*scaleDownThreshold

	if cpuBelowThreshold && memBelowThreshold {
		o.logger.Debug("load significantly below threshold - scale down",
			"service", service.ServiceName,
			"current_cpu", fmt.Sprintf("%.2f%%", currentMetrics.AvgCPUPercent),
			"current_mem", fmt.Sprintf("%.2f%%", currentMetrics.AvgMemoryPercent),
			"predicted_cpu", fmt.Sprintf("%.2f%%", predictedCPU),
			"predicted_mem", fmt.Sprintf("%.2f%%", predictedMem))
		return true
	}

	return false
}

// performScaleUp выполняет масштабирование вверх
func (o *Orchestrator) performScaleUp(
	ctx context.Context,
	service *types.ServiceConfig,
	metrics *types.ServiceMetrics,
	predictedCPU, predictedMem float64,
) {
	// Определяем, насколько агрессивно масштабировать
	scaleFactor := o.calculateScaleFactor(metrics, predictedCPU, predictedMem, service)

	currentReplicas := service.Replicas
	newReplicas := currentReplicas + scaleFactor

	// Не превышаем максимум
	if newReplicas > service.ScalePolicy.MaxReplicas {
		newReplicas = service.ScalePolicy.MaxReplicas
	}

	if newReplicas == currentReplicas {
		return // нечего масштабировать
	}

	o.logger.Info("predictive scale up triggered",
		"service", service.ServiceName,
		"from", currentReplicas,
		"to", newReplicas,
		"scale_factor", scaleFactor,
		"predicted_cpu", fmt.Sprintf("%.2f%%", predictedCPU),
		"predicted_mem", fmt.Sprintf("%.2f%%", predictedMem),
		"current_cpu", fmt.Sprintf("%.2f%%", metrics.AvgCPUPercent),
		"current_mem", fmt.Sprintf("%.2f%%", metrics.AvgMemoryPercent))

	service.Replicas = newReplicas

	// Создаём недостающие задачи
	for i := 0; i < scaleFactor; i++ {
		if err := o.createServiceTask(ctx, service); err != nil {
			o.logger.Error("failed to create task during predictive scale up",
				"service", service.ServiceName,
				"error", err)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// performScaleDown выполняет масштабирование вниз
func (o *Orchestrator) performScaleDown(
	ctx context.Context,
	service *types.ServiceConfig,
	metrics *types.ServiceMetrics,
	predictedCPU, predictedMem float64,
) {
	currentReplicas := service.Replicas

	// Масштабируем вниз на 1 реплику за раз (консервативно)
	newReplicas := currentReplicas - 1

	if newReplicas < service.ScalePolicy.MinReplicas {
		newReplicas = service.ScalePolicy.MinReplicas
	}

	if newReplicas == currentReplicas {
		return
	}

	o.logger.Info("predictive scale down triggered",
		"service", service.ServiceName,
		"from", currentReplicas,
		"to", newReplicas,
		"predicted_cpu", fmt.Sprintf("%.2f%%", predictedCPU),
		"predicted_mem", fmt.Sprintf("%.2f%%", predictedMem),
		"current_cpu", fmt.Sprintf("%.2f%%", metrics.AvgCPUPercent),
		"current_mem", fmt.Sprintf("%.2f%%", metrics.AvgMemoryPercent))

	service.Replicas = newReplicas

	// Используем существующий scaleDownService для остановки одной задачи
	tasks, err := o.taskStore.ListByService(ctx, service.ServiceName)
	if err != nil {
		o.logger.Error("failed to list tasks for scale down", "error", err)
		return
	}

	o.scaleDownService(ctx, service, tasks, 1)
}

// calculateScaleFactor определяет коэффициент масштабирования
func (o *Orchestrator) calculateScaleFactor(
	metrics *types.ServiceMetrics,
	predictedCPU, predictedMem float64,
	service *types.ServiceConfig,
) int {
	config := service.ScalePolicy.PredictiveScaling

	// Вычисляем, насколько текущая/предсказанная нагрузка превышает порог
	cpuRatio := math.Max(metrics.AvgCPUPercent, predictedCPU) / config.CPUThreshold
	memRatio := math.Max(metrics.AvgMemoryPercent, predictedMem) / config.MemoryThreshold

	// Берём максимальное отклонение
	maxRatio := math.Max(cpuRatio, memRatio)

	// Определяем количество реплик для добавления
	switch {
	case maxRatio > 2.0:
		return 3 // Очень высокая нагрузка - добавляем 3 реплики
	case maxRatio > 1.5:
		return 2 // Высокая нагрузка - добавляем 2 реплики
	case maxRatio > 1.0:
		return 1 // Умеренная нагрузка - добавляем 1 реплику
	default:
		return 1 // Консервативно - добавляем 1 реплику
	}
}

// Простое предсказание CPU на основе линейного тренда
func (o *Orchestrator) predictCPU(history []types.ContainerMetric, predictionWindow int) float64 {
	if len(history) < 2 {
		return 0
	}

	// Берем последние N записей
	recentHistory := history
	if len(history) > 10 {
		recentHistory = history[len(history)-10:]
	}

	// Вычисляем тренд
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(recentHistory))

	for i, m := range recentHistory {
		x := float64(i)
		y := m.CPUPercent
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Линейная регрессия: y = a + b*x
	b := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	a := (sumY - b*sumX) / n

	// Предсказываем через predictionWindow/10 шагов
	futurePoint := n + float64(predictionWindow)/10.0
	predicted := a + b*futurePoint

	if predicted < 0 {
		predicted = 0
	}
	if predicted > 100 {
		predicted = 100
	}

	return predicted
}

func (o *Orchestrator) predictMemory(history []types.ContainerMetric, predictionWindow int) float64 {
	// Аналогично predictCPU но для памяти
	if len(history) < 2 {
		return 0
	}

	recentHistory := history
	if len(history) > 10 {
		recentHistory = history[len(history)-10:]
	}

	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(recentHistory))

	for i, m := range recentHistory {
		x := float64(i)
		y := m.MemoryPercent
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	b := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	a := (sumY - b*sumX) / n

	futurePoint := n + float64(predictionWindow)/10.0
	predicted := a + b*futurePoint

	if predicted < 0 {
		predicted = 0
	}
	if predicted > 100 {
		predicted = 100
	}

	return predicted
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
func (o *Orchestrator) checkHealth() {
	ctx := o.ctx

	tasks, err := o.taskStore.ListByStatus(ctx, types.TaskStatusRunning)
	if err != nil {
		o.logger.Error("failed to list running tasks for health check", "error", err)
		return
	}

	o.logger.Debug("checkHealth: found running tasks", "count", len(tasks))

	if len(tasks) == 0 {
		o.logger.Warn("checkHealth: no running tasks found - health check skipped")
		return
	}

	checkedCount := 0
	unhealthyCount := 0
	errorCount := 0

	for _, task := range tasks {
		// Пропускаем задачи без конфигурации health check
		if task.ServiceConfig == nil || task.ServiceConfig.HealthCheck == nil {
			o.logger.Debug("checkHealth: task has no health check config", "task_id", task.ID)
			continue
		}

		// Проверяем статус контейнера
		status, err := o.dockerClient.GetConatinerStatus(ctx, task.ContainerID)
		if err != nil {
			o.logger.Error("checkHealth: failed to get container status",
				"task_id", task.ID,
				"container_id", task.ContainerID[:12],
				"error", err)
			errorCount++
			continue
		}

		if status != "running" && status != "running_healthy" && status != "starting" {
			o.logger.Warn("checkHealth: container not running",
				"task_id", task.ID,
				"status", status)

			task.Status = types.TaskStatusFailed
			now := time.Now()
			task.FinishedAt = &now
			o.taskStore.Update(ctx, task)

			if task.NodeID != "" {
				o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
			}
			unhealthyCount++
			continue
		}

		// Проверяем health
		healthy, err := o.dockerClient.CheckContainerHealth(ctx, task.ContainerID, task.ServiceConfig.HealthCheck)
		if err != nil {
			o.logger.Error("checkHealth: health check error",
				"task_id", task.ID,
				"error", err)
			errorCount++
			continue
		}

		checkedCount++

		if !healthy {
			o.logger.Warn("checkHealth: container unhealthy",
				"task_id", task.ID,
				"service", task.ServiceName)

			task.Status = types.TaskStatusFailed
			now := time.Now()
			task.FinishedAt = &now
			o.taskStore.Update(ctx, task)

			if task.NodeID != "" {
				o.scheduler.ReleaseNodeResources(ctx, task.NodeID, task)
			}
			unhealthyCount++
		}
	}

	o.logger.Info("checkHealth completed",
		"total_running", len(tasks),
		"checked", checkedCount,
		"unhealthy", unhealthyCount,
		"errors", errorCount)
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
