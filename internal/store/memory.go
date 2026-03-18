package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/exitae337/gorchester/internal/types"
)

// TaskStore interface realization

// In-memory struct -> store for Tasks
type MemoryTaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*types.Task
	// Fast search
	byContainerID map[string]string                    // containerID -> taskID
	byNodeID      map[string]map[string]bool           // nodeID -> set of TaskID
	byServiceName map[string]map[string]bool           // serviceName -> set of TaskID
	byStatus      map[types.TaskStatus]map[string]bool // status -> set of TaskID
}

// New MemoryTaskStore object -> New() -> Constructor
func New() *MemoryTaskStore {
	return &MemoryTaskStore{
		tasks:         make(map[string]*types.Task),
		byContainerID: make(map[string]string),
		byNodeID:      make(map[string]map[string]bool),
		byServiceName: make(map[string]map[string]bool),
		byStatus:      make(map[types.TaskStatus]map[string]bool),
	}
}

// Create -> save New Task in store
func (mem *MemoryTaskStore) Create(ctx context.Context, task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task can't be nil")
	}

	if task.ID == "" {
		return fmt.Errorf("task ID can't be empty")
	}

	// Lock mutex
	mem.mu.Lock()
	defer mem.mu.Unlock()

	if _, exists := mem.tasks[task.ID]; exists {
		return fmt.Errorf("task with ID %s already exists", task.ID)
	}

	// Set flags
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}

	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now()
	}

	mem.tasks[task.ID] = task

	mem.updateIndices(task)

	return nil
}

// Get -> get task By ID
func (mem *MemoryTaskStore) Get(ctx context.Context, id string) (*types.Task, error) {
	if id == "" {
		return nil, fmt.Errorf("task ID can't be nil")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	task, exists := mem.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task with id %s is not exists", id)
	}

	return task.DeepCopy(), nil
}

// Update -> update current Task
func (mem *MemoryTaskStore) Update(ctx context.Context, task *types.Task) error {
	if task == nil {
		return fmt.Errorf("task can't be nil for updating")
	}

	if task.ID == "" {
		return fmt.Errorf("task ID can't be nil while updating")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	existing, exist := mem.tasks[task.ID]
	if !exist {
		return fmt.Errorf("task with ID %s is not exists", task.ID)
	}

	task.UpdatedAt = time.Now()

	// New indexes
	oldContainerID := existing.ContainerID
	oldNodeID := existing.NodeID
	oldServiceName := existing.ServiceName
	oldStatus := existing.Status

	mem.tasks[task.ID] = task

	mem.updateIndicesWithOldValues(task, oldContainerID, oldNodeID, oldServiceName, oldStatus)

	return nil
}

func (mem *MemoryTaskStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id can't be empty while deleting")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	task, exists := mem.tasks[id]
	if !exists {
		fmt.Errorf("task with ID %s is not exists", id)
	}

	delete(mem.tasks, id)

	mem.removeFromIndices(task)

	return nil
}

// List all tasks
func (mem *MemoryTaskStore) List(ctx context.Context) ([]*types.Task, error) {
	mem.mu.Lock()
	defer mem.mu.Unlock()

	tasks := make([]*types.Task, 0, len(mem.tasks))
	for _, task := range mem.tasks {
		tasks = append(tasks, task.DeepCopy())
	}

	return tasks, nil
}

// ListByService -> all Tasks by Service
func (mem *MemoryTaskStore) ListByService(ctx context.Context, serviceName string) ([]*types.Task, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("serviceName can't be nil")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	tasksID, exists := mem.byServiceName[serviceName]
	if !exists {
		return []*types.Task{}, nil
	}

	tasks := make([]*types.Task, 0, len(tasksID))
	for taskID := range tasksID {
		if task, exists := mem.tasks[taskID]; exists {
			tasks = append(tasks, task.DeepCopy())
		}
	}

	return tasks, nil
}

// ListByStatus -> return all Tasks with 'status'
func (mem *MemoryTaskStore) ListByStatus(ctx context.Context, status types.TaskStatus) ([]*types.Task, error) {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	taskIDs, exists := mem.byStatus[status]
	if !exists {
		return []*types.Task{}, nil
	}

	tasks := make([]*types.Task, 0, len(taskIDs))
	for taskID := range taskIDs {
		if task, exists := mem.tasks[taskID]; exists {
			tasks = append(tasks, task.DeepCopy())
		}
	}
	return tasks, nil
}

// ListByNodeID -> all tasks on 'nodeID'
func (mem *MemoryTaskStore) ListByNodeID(ctx context.Context, nodeID string) ([]*types.Task, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("node ID cannot be empty")
	}

	mem.mu.RLock()
	defer mem.mu.RUnlock()

	taskIDs, exists := mem.byNodeID[nodeID]
	if !exists {
		return []*types.Task{}, nil
	}

	tasks := make([]*types.Task, 0, len(taskIDs))
	for taskID := range taskIDs {
		if task, exists := mem.tasks[taskID]; exists {
			tasks = append(tasks, task.DeepCopy())
		}
	}
	return tasks, nil
}

// Count -> tasks count
func (mem *MemoryTaskStore) Count(ctx context.Context) (int, error) {
	mem.mu.RLock()
	defer mem.mu.RUnlock()
	return len(mem.tasks), nil
}

// CountByService -> tasks count by Service
func (mem *MemoryTaskStore) CountByService(ctx context.Context, serviceName string) (int, error) {
	if serviceName == "" {
		return 0, fmt.Errorf("service name cannot be empty")
	}

	mem.mu.RLock()
	defer mem.mu.RUnlock()

	taskIDs, exists := mem.byServiceName[serviceName]
	if !exists {
		return 0, nil
	}
	return len(taskIDs), nil
}

// CountByStatus -> count by task status
func (mem *MemoryTaskStore) CountByStatus(ctx context.Context, status types.TaskStatus) (int, error) {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	taskIDs, exists := mem.byStatus[status]
	if !exists {
		return 0, nil
	}
	return len(taskIDs), nil
}

// GetByContainerID -> task by Container ID
func (mem *MemoryTaskStore) GetByContainerID(ctx context.Context, containerID string) (*types.Task, error) {
	if containerID == "" {
		return nil, fmt.Errorf("container ID cannot be empty")
	}

	mem.mu.RLock()
	defer mem.mu.RUnlock()

	taskID, exists := mem.byContainerID[containerID]
	if !exists {
		return nil, fmt.Errorf("task for container %s not found", containerID)
	}

	task, exists := mem.tasks[taskID]
	if !exists {
		// maybe...
		delete(mem.byContainerID, containerID)
		return nil, fmt.Errorf("inconsistent state: task %s not found for container %s", taskID, containerID)
	}

	return task.DeepCopy(), nil
}

// TaskStats -> task Stats
func (mem *MemoryTaskStore) TaskStats(ctx context.Context, id string) (*types.TaskStats, error) {
	task, err := mem.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	stats := &types.TaskStats{
		RestartCount:  task.RestartCount,
		CurrentStatus: task.Status,
		CPUUsage:      float64(task.CPUUsage),
		MemoryUsage:   task.MemoryUsage,
	}

	// Count uptime if task is running
	if task.IsRunning() && task.StartedAt != nil {
		stats.Uptime = time.Since(*task.StartedAt)
	}

	return stats, nil
}

// UpdateMany -> update many tasks
func (mem *MemoryTaskStore) UpdateMany(ctx context.Context, tasks []types.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	for i := range tasks {
		task := &tasks[i]
		if _, exists := mem.tasks[task.ID]; !exists {
			return fmt.Errorf("task with ID %s not found", task.ID)
		}
	}

	// Update if all tasks exist
	for i := range tasks {
		task := &tasks[i]
		existing := mem.tasks[task.ID]

		// Old values for index updating
		oldContainerID := existing.ContainerID
		oldNodeID := existing.NodeID
		oldServiceName := existing.ServiceName
		oldStatus := existing.Status

		task.UpdatedAt = time.Now()
		mem.tasks[task.ID] = task

		mem.updateIndicesWithOldValues(task, oldContainerID, oldNodeID, oldServiceName, oldStatus)
	}

	return nil
}

// UpdateStatus -> update only task status
func (mem *MemoryTaskStore) UpdateStatus(ctx context.Context, id string, status types.TaskStatus) error {
	if id == "" {
		return fmt.Errorf("task ID cannot be empty")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	task, exists := mem.tasks[id]
	if !exists {
		return fmt.Errorf("task with ID %s not found", id)
	}

	oldStatus := task.Status

	task.Status = status
	task.UpdatedAt = time.Now()

	// Update Index by status
	mem.updateStatusIndex(task, oldStatus)

	return nil
}

// IncrementRestartCounter -> Restart counter++
func (mem *MemoryTaskStore) IncrementRestartCounter(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("task ID cannot be empty")
	}

	mem.mu.Lock()
	defer mem.mu.Unlock()

	task, exists := mem.tasks[id]
	if !exists {
		return fmt.Errorf("task with ID %s not found", id)
	}

	task.RestartCount++
	task.UpdatedAt = time.Now()

	return nil
}

// ========== UTIL FUNCS FOR INDEXES ==========

// updateIndices -> Update Indexes for Task
func (s *MemoryTaskStore) updateIndices(task *types.Task) {
	if task.ContainerID != "" {
		s.byContainerID[task.ContainerID] = task.ID
	}

	if task.NodeID != "" {
		if _, exists := s.byNodeID[task.NodeID]; !exists {
			s.byNodeID[task.NodeID] = make(map[string]bool)
		}
		s.byNodeID[task.NodeID][task.ID] = true
	}

	if task.ServiceName != "" {
		if _, exists := s.byServiceName[task.ServiceName]; !exists {
			s.byServiceName[task.ServiceName] = make(map[string]bool)
		}
		s.byServiceName[task.ServiceName][task.ID] = true
	}

	if _, exists := s.byStatus[task.Status]; !exists {
		s.byStatus[task.Status] = make(map[string]bool)
	}
	s.byStatus[task.Status][task.ID] = true
}

// updateIndicesWithOldValues -> Update Indexes by values
func (s *MemoryTaskStore) updateIndicesWithOldValues(task *types.Task, oldContainerID, oldNodeID, oldServiceName string, oldStatus types.TaskStatus) {
	if oldContainerID != task.ContainerID {
		if oldContainerID != "" {
			delete(s.byContainerID, oldContainerID)
		}
		if task.ContainerID != "" {
			s.byContainerID[task.ContainerID] = task.ID
		}
	}

	if oldNodeID != task.NodeID {
		if oldNodeID != "" {
			delete(s.byNodeID[oldNodeID], task.ID)
			if len(s.byNodeID[oldNodeID]) == 0 {
				delete(s.byNodeID, oldNodeID)
			}
		}
		if task.NodeID != "" {
			if _, exists := s.byNodeID[task.NodeID]; !exists {
				s.byNodeID[task.NodeID] = make(map[string]bool)
			}
			s.byNodeID[task.NodeID][task.ID] = true
		}
	}

	if oldServiceName != task.ServiceName {
		if oldServiceName != "" {
			delete(s.byServiceName[oldServiceName], task.ID)
			if len(s.byServiceName[oldServiceName]) == 0 {
				delete(s.byServiceName, oldServiceName)
			}
		}
		if task.ServiceName != "" {
			if _, exists := s.byServiceName[task.ServiceName]; !exists {
				s.byServiceName[task.ServiceName] = make(map[string]bool)
			}
			s.byServiceName[task.ServiceName][task.ID] = true
		}
	}

	s.updateStatusIndex(task, oldStatus)
}

// updateStatusIndex -> update only index by status
func (s *MemoryTaskStore) updateStatusIndex(task *types.Task, oldStatus types.TaskStatus) {
	if oldStatus != task.Status {
		if oldStatus != "" {
			delete(s.byStatus[oldStatus], task.ID)
			if len(s.byStatus[oldStatus]) == 0 {
				delete(s.byStatus, oldStatus)
			}
		}

		if task.Status != "" {
			if _, exists := s.byStatus[task.Status]; !exists {
				s.byStatus[task.Status] = make(map[string]bool)
			}
			s.byStatus[task.Status][task.ID] = true
		}
	}
}

// removeFromIndices -> delete Task from all Indexes
func (s *MemoryTaskStore) removeFromIndices(task *types.Task) {
	if task.ContainerID != "" {
		delete(s.byContainerID, task.ContainerID)
	}

	if task.NodeID != "" {
		delete(s.byNodeID[task.NodeID], task.ID)
		if len(s.byNodeID[task.NodeID]) == 0 {
			delete(s.byNodeID, task.NodeID)
		}
	}

	if task.ServiceName != "" {
		delete(s.byServiceName[task.ServiceName], task.ID)
		if len(s.byServiceName[task.ServiceName]) == 0 {
			delete(s.byServiceName, task.ServiceName)
		}
	}

	if task.Status != "" {
		delete(s.byStatus[task.Status], task.ID)
		if len(s.byStatus[task.Status]) == 0 {
			delete(s.byStatus, task.Status)
		}
	}
}
