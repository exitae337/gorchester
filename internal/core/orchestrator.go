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

// Scheduler определяет интерфейс планировщика
type Scheduler interface {
	// SelectNode выбирает узел для задачи на основе требований
	SelectNode(ctx context.Context, task *Task, nodes []*types.Node) (string, error)

	// GetNodes возвращает список доступных узлов
	GetNodes(ctx context.Context) ([]*types.Node, error)

	// RegisterNode регистрирует новый узел в кластере
	RegisterNode(ctx context.Context, node *types.Node) error

	// UnregisterNode удаляет узел из кластера
	UnregisterNode(ctx context.Context, nodeID string) error

	// UpdateNodeStatus обновляет статус узла
	UpdateNodeStatus(ctx context.Context, nodeID string, status types.NodeStatus) error
}

// Store interface
type TaskStore interface {
	// Create new Task
	Create(ctx context.Context, task *Task) error
	// Get Task by ID
	Get(ctx context.Context, id string) (*Task, error)
	// Update Task
	Update(ctx context.Context, task *Task) error
	// Delete Task
	Delete(ctx context.Context, id string) error

	// List all Tasks
	List(ctx context.Context) ([]*Task, error)
	// List all Tasks by Service ID
	ListByService(ctx context.Context, serviceName string) ([]*Task, error)
	// List Tasks by status
	ListByStatus(ctx context.Context, status TaskStatus) ([]*Task, error)
	// Get by Node ID
	ListByNodeID(ctx context.Context, nodeID string) ([]*Task, error)

	// Count all tasks
	Count(ctx context.Context) (int, error)
	// Count Tasks by service
	CountByService(ctx context.Context, serviceID string) (int, error)
	// Count by status
	CountByStatus(ctx context.Context, status TaskStatus) (int, error)

	// Get Task by container ID
	GetByContainerID(ctx context.Context, containerID string) (*Task, error)
	// Get task Stats
	TaskStats(ctx context.Context, id string) (*TaskStats, error)
	// Update several Tasks
	UpdateMany(ctx context.Context, tasks []Task) error
	// Update Task status
	UpdateStatus(ctx context.Context, id string, status TaskStatus) error
	// Increment Restart Counter
	IncrementRestartCounter(ctx context.Context, id string) error
}

// Task stats -> Struct for Task struct
type TaskStats struct {
	Uptime        time.Duration `json:"uptime"`
	RestartCount  int           `json:"restart_counter"`
	CPUUsage      float64       `json:"cpu_usage"`
	MemoryUsage   int64         `json:"memory_usage"`
	CurrentStatus TaskStatus    `json:"current_status"`
}

// Orchestrator settings
type OrchestratorSettings struct {
	ReconcileInterval   time.Duration // reconcile interval
	HealthCheckInterval time.Duration // health check interval
}

// DefaultOrchestrator Settings
func DefaultOrchestratorSettings() *OrchestratorSettings {
	return &OrchestratorSettings{
		ReconcileInterval:   30 * time.Second,
		HealthCheckInterval: 15 * time.Second,
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

// Start Orchestrator !
func (o *Orchestrator) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.isRunning {
		return fmt.Errorf("orchestrator is already running")
	}

	o.ctx, o.cancel = context.WithCancel(context.Background())
	o.isRunning = true

	// Background cycles
	o.wg.Add(2)

	// TODO -> Cycles

	return nil
}

// Start services by init app Config
func (o *Orchestrator) initServices() error {
	ctx := context.Background()

	for _, svc := range o.appConfig.Services {
		o.logger.Info("initializing service",
			"service", svc.ServiceName,
			"replicas", svc.Replicas)

		// Проверяем, сколько задач уже есть для этого сервиса
		existingTasks, err := o.taskStore.ListByService(ctx, svc.ServiceName)
		if err != nil {
			return fmt.Errorf("failed to list tasks for service %s: %w", svc.ServiceName, err)
		}

		// Создаем недостающие реплики
		for i := len(existingTasks); i < svc.Replicas; i++ {
			if err := o.createServiceTask(ctx, &svc); err != nil {
				o.logger.Error("failed to create task during init",
					"service", svc.ServiceName,
					"error", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

// Create service Task
func (o *Orchestrator) createServiceTask(ctx context.Context, service *types.ServiceConfig) error {
	taskID := uuid.New().String()

	// Выбираем узел для задачи
	nodes, err := o.scheduler.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// Создаем временный объект Task только для планировщика
	tempTask := &Task{
		ID:            taskID,
		ServiceName:   service.ServiceName,
		ServiceConfig: service,
	}

	nodeID, err := o.scheduler.SelectNode(ctx, tempTask, nodes)
	if err != nil {
		return fmt.Errorf("failed to select node: %w", err)
	}

	// Создаем полноценную задачу
	now := time.Now()
	task := &Task{
		ID:            taskID,
		ServiceName:   service.ServiceName,
		Status:        TaskStatusPending,
		DesiredState:  TaskStatusRunning,
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

	// Сохраняем в хранилище
	if err := o.taskStore.Create(ctx, task); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	o.logger.Info("task created and saved",
		"task_id", taskID,
		"service", service.ServiceName,
		"node", nodeID)

	// Запускаем выполнение асинхронно
	go o.executeTask(task)

	return nil
}

// executeTask выполняет задачу: создает и запускает контейнер через Docker
func (o *Orchestrator) executeTask(task *Task) {
	// Используем отдельный контекст для этой операции, но привязываем к общему o.ctx,
	// чтобы при остановке оркестратора все операции прерывались
	ctx, cancel := context.WithCancel(o.ctx)
	defer cancel()

	taskLogger := o.logger.With(
		"task_id", task.ID,
		"service", task.ServiceName,
	)

	taskLogger.Info("executing task")

	// 1. Обновляем статус на "starting" (можно добавить промежуточный статус, если нужно)
	// Для простоты оставим pending, но можно создать TaskStatusStarting

	// 2. Вызываем ваш DockerClient для создания и запуска контейнера
	containerID, err := o.dockerClient.CreateContainer(
		ctx,
		task.ServiceConfig,
		task.ID,
		taskLogger, // Передаем логгер с контекстом задачи
	)

	if err != nil {
		taskLogger.Error("failed to create/start container", "error", err)
		// Обновляем статус задачи на failed
		task.UpdateTask(TaskStatusFailed)
		task.Error = err.Error()
		if updateErr := o.taskStore.Update(ctx, task); updateErr != nil {
			taskLogger.Error("failed to update task status to failed", "error", updateErr)
		}
		return
	}

	// 3. Успех — обновляем задачу
	task.ContainerID = containerID
	task.UpdateTask(TaskStatusRunning) // Устанавливает статус и StartedAt

	if err := o.taskStore.Update(ctx, task); err != nil {
		taskLogger.Error("failed to update task status to running", "error", err)
		// Контейнер уже запущен, но мы не смогли обновить статус. Это проблема.
		// В реальности нужно попытаться остановить контейнер или отметить для перезапуска.
		return
	}

	taskLogger.Info("container started successfully", "container_id", containerID[:12])
}

// reconcileLoop периодически запускает reconcile
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

// reconcile проверяет и исправляет состояние кластера
func (o *Orchestrator) reconcile() {
	ctx := context.Background()
	o.logger.Debug("starting reconciliation")

	// 1. Получаем все задачи
	tasks, err := o.taskStore.List(ctx)
	if err != nil {
		o.logger.Error("failed to list tasks", "error", err)
		return
	}

	// 2. Группируем задачи по сервисам для удобства
	tasksByService := make(map[string][]*Task)
	for _, task := range tasks {
		tasksByService[task.ServiceName] = append(tasksByService[task.ServiceName], task)
	}

	// 3. Для каждого сервиса из конфигурации
	for _, svc := range o.appConfig.Services {
		serviceTasks := tasksByService[svc.ServiceName]
		if serviceTasks == nil {
			serviceTasks = []*Task{}
		}

		// Подсчитываем задачи в разных состояниях
		var running, pending, failed, stopped int
		for _, t := range serviceTasks {
			switch t.Status {
			case TaskStatusRunning:
				running++
			case TaskStatusPending:
				pending++
			case TaskStatusFailed:
				failed++
			case TaskStatusStopped:
				stopped++
			}
		}

		// 4. Проверяем задачи, которые нужно перезапустить (NeedsRestart)
		for _, task := range serviceTasks {
			if task.NeedsRestart() {
				o.logger.Info("task needs restart",
					"task_id", task.ID,
					"status", task.Status,
					"desired", task.DesiredState)

				// Увеличиваем счетчик перезапусков
				o.taskStore.IncrementRestartCounter(ctx, task.ID)

				// Пытаемся перезапустить (создаем новую задачу, старую позже почистим)
				// В простейшем случае — создаем новую реплику
				if err := o.createServiceTask(ctx, &svc); err != nil {
					o.logger.Error("failed to create replacement task", "error", err)
				}
			}
		}

		// 5. Проверяем количество реплик с учетом scale policy
		desiredReplicas := svc.Replicas // Базовое значение из конфига
		if svc.ScalePolicy.MinReplicas > 0 {
			// Если задан min_replicas, используем его как нижнюю границу
			desiredReplicas = svc.ScalePolicy.MinReplicas
		}

		// Здесь позже будет логика автоскейлинга на основе метрик

		currentReplicas := running + pending // Считаем, что pending скоро станут running

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
				time.Sleep(100 * time.Millisecond)
			}
		} else if currentReplicas > desiredReplicas && svc.ScalePolicy.MaxReplicas > 0 && currentReplicas > svc.ScalePolicy.MaxReplicas {
			// Если превышен max_replicas — нужно уменьшить
			excess := currentReplicas - svc.ScalePolicy.MaxReplicas
			o.logger.Info("scaling down (exceeds max)",
				"service", svc.ServiceName,
				"current", currentReplicas,
				"max", svc.ScalePolicy.MaxReplicas,
				"excess", excess)
			// TODO: реализовать остановку лишних задач
		}
	}

	o.logger.Debug("reconciliation completed")
}

// healthCheckLoop периодически запускает checkHealth
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

// checkHealth проверяет здоровье запущенных контейнеров
// ВАЖНО: Ваш DockerClient пока не имеет метода для проверки здоровья.
// Эта функция будет заглушкой, которую нужно развивать.
func (o *Orchestrator) checkHealth() {
	ctx := context.Background()

	// Получаем все запущенные задачи
	tasks, err := o.taskStore.ListByStatus(ctx, TaskStatusRunning)
	if err != nil {
		o.logger.Error("failed to list running tasks", "error", err)
		return
	}

	for _, task := range tasks {
		// Пропускаем задачи без health check
		if task.ServiceConfig == nil {
			continue
		}

		// TODO: Реализовать реальную проверку здоровья.
		// Для этого нужно расширить DockerClient методом вроде:
		// InspectContainer или Exec, чтобы проверить процесс внутри.
		// Пока просто считаем все контейнеры здоровыми.
		_ = task.ServiceConfig.HealthCheck // Заглушка, чтобы избежать предупреждения

		// Пример того, как это будет выглядеть в будущем:
		// healthy, err := o.dockerClient.CheckContainerHealth(ctx, task.ContainerID, task.ServiceConfig.HealthCheck)
		// if err != nil {
		//     o.logger.Error("health check failed", "task_id", task.ID, "error", err)
		//     task.UpdateTask(TaskStatusFailed)
		//     o.taskStore.Update(ctx, task)
		// }
	}

	o.logger.Debug("health check completed", "checked_count", len(tasks))
}

// GetTask возвращает задачу по ID (для API)
func (o *Orchestrator) GetTask(ctx context.Context, id string) (*Task, error) {
	return o.taskStore.Get(ctx, id)
}

// ListTasks возвращает все задачи (для API)
func (o *Orchestrator) ListTasks(ctx context.Context) ([]*Task, error) {
	return o.taskStore.List(ctx)
}

// DeleteTask удаляет задачу (для API)
func (o *Orchestrator) DeleteTask(ctx context.Context, id string) error {
	task, err := o.taskStore.Get(ctx, id)
	if err != nil {
		return err
	}

	// Если контейнер запущен — останавливаем и удаляем его
	if task.ContainerID != "" {
		if err := o.dockerClient.StopContainer(ctx, task.ContainerID); err != nil {
			o.logger.Warn("failed to stop container during task deletion",
				"task_id", id,
				"container", task.ContainerID[:12],
				"error", err)
			// Продолжаем, пытаемся удалить задачу из store
		}
		if err := o.dockerClient.RemoveContainer(ctx, task.ContainerID); err != nil {
			o.logger.Warn("failed to remove container during task deletion",
				"task_id", id,
				"container", task.ContainerID[:12],
				"error", err)
		}
	}

	return o.taskStore.Delete(ctx, id)
}
