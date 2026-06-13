package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/exitae337/gorchester/internal/core"
	"github.com/exitae337/gorchester/internal/metrics"
	"github.com/exitae337/gorchester/internal/scheduler"
	"github.com/exitae337/gorchester/internal/types"
)

type APIServer struct {
	orch    *core.Orchestrator
	sched   *scheduler.SimpleScheduler
	metrics *metrics.MetricsStore
	logger  *slog.Logger
	mux     *http.ServeMux
}

func NewAPIServer(
	orch *core.Orchestrator,
	sched *scheduler.SimpleScheduler,
	metricsStore *metrics.MetricsStore,
	logger *slog.Logger,
) *APIServer {
	s := &APIServer{
		orch:    orch,
		sched:   sched,
		metrics: metricsStore,
		logger:  logger.With("component", "api"),
		mux:     http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *APIServer) registerRoutes() {
	// Health
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Services
	s.mux.HandleFunc("/api/v1/services", s.handleServices)
	s.mux.HandleFunc("/api/v1/services/", s.handleServiceByPath)

	// Nodes
	s.mux.HandleFunc("/api/v1/nodes", s.handleNodes)

	// Tasks
	s.mux.HandleFunc("/api/v1/tasks", s.handleTasks)

	// Metrics
	s.mux.HandleFunc("/api/v1/metrics", s.handleMetrics)

	// Config strategy
	s.mux.HandleFunc("/api/v1/config/strategy", s.handleStrategy)

	// Change Node Status
	s.mux.HandleFunc("/api/v1/nodes/", s.handleNodeStatusByPath)
}

// Start API Server
func (s *APIServer) Start(addr string) error {
	s.logger.Info("API server starting", "addr", addr)
	return http.ListenAndServe(addr, s.mux)
}

// Health check
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}

// Services check
func (s *APIServer) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()
	tasks, err := s.orch.ListTasks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Group by service
	services := make(map[string]map[string]interface{})
	for _, task := range tasks {
		if _, exists := services[task.ServiceName]; !exists {
			services[task.ServiceName] = map[string]interface{}{
				"service_name": task.ServiceName,
				"running":      0,
				"pending":      0,
				"failed":       0,
				"stopped":      0,
			}
		}
		svc := services[task.ServiceName]
		switch task.Status {
		case types.TaskStatusRunning:
			svc["running"] = svc["running"].(int) + 1
		case types.TaskStatusPending:
			svc["pending"] = svc["pending"].(int) + 1
		case types.TaskStatusFailed:
			svc["failed"] = svc["failed"].(int) + 1
		case types.TaskStatusStopped:
			svc["stopped"] = svc["stopped"].(int) + 1
		}
	}

	result := make([]map[string]interface{}, 0, len(services))
	for _, svc := range services {
		result = append(result, svc)
	}

	writeJSON(w, http.StatusOK, result)
}

// Service by Path check
func (s *APIServer) handleServiceByPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse path: /api/v1/services/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "service name is required")
		return
	}

	serviceName := parts[0]

	// GET service info
	ctx := context.Background()
	tasks, err := s.orch.ListTasks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var serviceTasks []map[string]interface{}
	for _, task := range tasks {
		if task.ServiceName == serviceName {
			containerID := task.ContainerID
			if len(containerID) > 12 {
				containerID = containerID[:12]
			}
			serviceTasks = append(serviceTasks, map[string]interface{}{
				"task_id":       task.ID[:8],
				"status":        task.Status,
				"node_id":       task.NodeID,
				"container_id":  containerID,
				"restart_count": task.RestartCount,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service_name": serviceName,
		"tasks":        serviceTasks,
		"total":        len(serviceTasks),
	})

}

// Nodes Handler
func (s *APIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()
	nodes, err := s.sched.GetNodes(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		cpuPct := float64(node.UsedCPU) / float64(node.Resources.CPU) * 100
		memPct := float64(node.UsedMemory) / float64(node.Resources.Memory) * 100

		result = append(result, map[string]interface{}{
			"id":           node.ID,
			"hostname":     node.Hostname,
			"status":       node.Status,
			"cpu_cores":    node.Resources.CPU / 1000,
			"cpu_used_pct": cpuPct,
			"mem_total_mb": node.Resources.Memory / 1024 / 1024,
			"mem_used_pct": memPct,
			"task_count":   node.TaskCount,
			"last_seen":    node.LastSeen.Format("2006-01-02T15:04:05"),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// Tasks handler
func (s *APIServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()
	tasks, err := s.orch.ListTasks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		containerID := task.ContainerID
		if len(containerID) > 12 {
			containerID = containerID[:12]
		}
		result = append(result, map[string]interface{}{
			"task_id":       task.ID[:8],
			"service_name":  task.ServiceName,
			"status":        task.Status,
			"desired_state": task.DesiredState,
			"node_id":       task.NodeID,
			"container_id":  containerID,
			"restart_count": task.RestartCount,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// Metrics Handler
func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get service names from orchestrator
	ctx := context.Background()
	tasks, err := s.orch.ListTasks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	serviceNames := make(map[string]bool)
	for _, task := range tasks {
		serviceNames[task.ServiceName] = true
	}

	result := make(map[string]interface{})
	for name := range serviceNames {
		metrics, err := s.metrics.GetServiceMetrics(name, 1*60*1000000000)
		if err != nil {
			continue
		}
		result[name] = map[string]interface{}{
			"avg_cpu":    metrics.AvgCPUPercent,
			"avg_memory": metrics.AvgMemoryPercent,
			"containers": metrics.TotalContainers,
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// Change Node status Handler
func (s *APIServer) handleNodeStatusByPath(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/v1/nodes/{id}/drain or /api/v1/nodes/{id}/activate
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "path must be /nodes/{id}/drain or /nodes/{id}/activate")
		return
	}

	nodeID := parts[0]
	action := parts[1]

	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := context.Background()

	switch action {
	case "drain":
		err := s.sched.UpdateNodeStatus(ctx, nodeID, types.NodeStatusDraining)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "draining",
			"node":    nodeID,
			"message": "Node drained. Containers will be rescheduled on next reconcile.",
		})
	case "activate":
		err := s.sched.UpdateNodeStatus(ctx, nodeID, types.NodeStatusReady)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ready",
			"node":    nodeID,
			"message": "Node reactivated. Ready for scheduling.",
		})
	default:
		writeError(w, http.StatusBadRequest, "action must be 'drain' or 'activate'")
	}
}

// Scaling Strategy Handler. TODO !!!
func (s *APIServer) handleStrategy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return current strategy
		writeJSON(w, http.StatusOK, map[string]string{
			"strategy": "spread", // TODO: get from scheduler config
		})
	case http.MethodPut:
		// Change strategy
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		strategy, ok := body["strategy"]
		if !ok {
			writeError(w, http.StatusBadRequest, "strategy field is required")
			return
		}

		// Validate strategy
		validStrategies := map[string]bool{
			"random": true, "round_robin": true, "binpack": true,
			"spread": true, "least_tasks": true, "least_resource": true,
		}
		if !validStrategies[strategy] {
			writeError(w, http.StatusBadRequest, "invalid strategy: "+strategy)
			return
		}

		// TODO: update scheduler strategy
		writeJSON(w, http.StatusOK, map[string]string{
			"status":    "not_implemented",
			"message":   "strategy change via API is not yet implemented",
			"requested": strategy,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ========= HELPERS =========

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
