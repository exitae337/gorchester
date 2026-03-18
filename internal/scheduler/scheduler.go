package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/exitae337/gorchester/internal/types"
)

// Strategy -> Planning stategy
type Strategy string

const (
	// Random strategy
	StrategyRandom Strategy = "random"

	// StrategyRoundRobin - round-robin
	StrategyRoundRobin Strategy = "round_robin"

	// StrategyBinpack - maximum load
	StrategyBinpack Strategy = "binpack"

	// StrategySpread - uniform
	StrategySpread Strategy = "spread"

	// StrategyLeastTasks - min count of tasks
	StrategyLeastTasks Strategy = "least_tasks"

	// StrategyLeastResource - max count of resources
	StrategyLeastResource Strategy = "least_resource"
)

// Scheduler config -> Local structure for Scheduler
type SchedulerConfig struct {
	Strategy           Strategy      // strategy
	HeartbeatTimeout   time.Duration // timeout of heartbeat
	CleanupInterval    time.Duration // delete (cleanup)
	ResourceOvercommit float64       // koef overcommit (1.0 = 100%)
}

// DefaultConfig -> default
func DefaultConfig() *SchedulerConfig {
	return &SchedulerConfig{
		Strategy:           StrategySpread,
		HeartbeatTimeout:   30 * time.Second,
		CleanupInterval:    1 * time.Minute,
		ResourceOvercommit: 1.0, // no overcommit
	}
}

// SimpleScheduler with different strategy
type SimpleScheduler struct {
	config *SchedulerConfig
	nodes  map[string]*types.Node
	mu     sync.RWMutex

	// Round-robin
	roundRobinIndex map[string]int // serviceName -> last index

	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NEW() -> constructor for scheduler()
func New(config *SchedulerConfig, logger *slog.Logger) *SimpleScheduler {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &SimpleScheduler{
		config:          config,
		nodes:           make(map[string]*types.Node),
		roundRobinIndex: make(map[string]int),
		logger:          logger.With("component", "scheduler"),
		ctx:             ctx,
		cancel:          cancel,
	}

	// FOR TESTING!!!
	s.addTestNodes()

	// Clean Up LOOP
	s.wg.Add(1)
	go s.cleanupLoop()

	return s
}

// Stop -> Scheduler stop
func (s *SimpleScheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Test nodes for development -> TODO !!! -> FOR TESTING
func (s *SimpleScheduler) addTestNodes() {
	testNodes := []*types.Node{
		{
			ID:       "node-1",
			Hostname: "worker-1.local",
			IP:       "192.168.1.101",
			Status:   types.NodeStatusReady,
			Resources: &types.NodeResources{
				CPU:    4000,                    // 4 core
				Memory: 16 * 1024 * 1024 * 1024, // 16 GB
			},
			Labels: map[string]string{
				"region": "us-east",
				"zone":   "a",
				"disk":   "ssd",
			},
			LastSeen: time.Now(),
		},
		{
			ID:       "node-2",
			Hostname: "worker-2.local",
			IP:       "192.168.1.102",
			Status:   types.NodeStatusReady,
			Resources: &types.NodeResources{
				CPU:    8000,                    // 8 core
				Memory: 32 * 1024 * 1024 * 1024, // 32 GB
			},
			Labels: map[string]string{
				"region": "us-east",
				"zone":   "b",
				"disk":   "ssd",
			},
			LastSeen: time.Now(),
		},
		{
			ID:       "node-3",
			Hostname: "worker-3.local",
			IP:       "192.168.1.103",
			Status:   types.NodeStatusReady,
			Resources: &types.NodeResources{
				CPU:    2000,                   // 2 core
				Memory: 8 * 1024 * 1024 * 1024, // 8 GB
			},
			Labels: map[string]string{
				"region": "eu-west",
				"zone":   "a",
				"disk":   "hdd",
			},
			LastSeen: time.Now(),
		},
	}

	for _, node := range testNodes {
		s.nodes[node.ID] = node
	}
}

// Clean up -> Delete not working nodes
func (s *SimpleScheduler) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupInactiveNodes()
		}
	}
}

// cleanupInactiveNodes -> delete nodes that not send heartbeat for a while
func (s *SimpleScheduler) cleanupInactiveNodes() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	timeout := s.config.HeartbeatTimeout

	for id, node := range s.nodes {
		if now.Sub(node.LastSeen) > timeout {
			s.logger.Warn("node inactive, removing", "node_id", id, "last_seen", node.LastSeen)
			delete(s.nodes, id)
		}
	}
}

// SelectNode -> choose Node by chosen strategy
func (s *SimpleScheduler) SelectNode(ctx context.Context, task *types.Task, nodes []*types.Node) (string, error) {
	if len(nodes) == 0 {
		return "", errors.New("no nodes available")
	}

	// Only READY Nodes
	readyNodes := s.filterReadyNodes(nodes)
	if len(readyNodes) == 0 {
		return "", errors.New("no ready nodes")
	}

	// We have resources?
	feasibleNodes := s.filterFeasibleNodes(readyNodes, task)
	if len(feasibleNodes) == 0 {
		return "", fmt.Errorf("no nodes with sufficient resources for task (req CPU: %dm, Mem: %d)",
			task.ServiceConfig.Resources.CPUMilliCores,
			task.ServiceConfig.Resources.MemoryBytes)
	}

	// Choose Node
	var selectedNode *types.Node
	var err error

	switch s.config.Strategy {
	case StrategyRandom:
		selectedNode, err = s.selectRandom(feasibleNodes)
	case StrategyRoundRobin:
		selectedNode, err = s.selectRoundRobin(feasibleNodes, task.ServiceName)
	case StrategyBinpack:
		selectedNode, err = s.selectBinpack(feasibleNodes, task)
	case StrategySpread:
		selectedNode, err = s.selectSpread(feasibleNodes)
	case StrategyLeastTasks:
		selectedNode, err = s.selectLeastTasks(feasibleNodes)
	case StrategyLeastResource:
		selectedNode, err = s.selectLeastResource(feasibleNodes, task)
	default:
		selectedNode, err = s.selectSpread(feasibleNodes)
	}

	if err != nil {
		return "", err
	}

	// Update Node resources
	s.updateNodeResources(selectedNode.ID, task)

	s.logger.Debug("selected node",
		"strategy", s.config.Strategy,
		"node_id", selectedNode.ID,
		"task_id", task.ID,
		"service", task.ServiceName)

	return selectedNode.ID, nil
}

// filterReadyNodes -> Only READY Nodes
func (s *SimpleScheduler) filterReadyNodes(nodes []*types.Node) []*types.Node {
	result := make([]*types.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.Status == types.NodeStatusReady {
			result = append(result, node)
		}
	}
	return result
}

// filterFeasibleNodes -> only Nodes with resources
func (s *SimpleScheduler) filterFeasibleNodes(nodes []*types.Node, task *types.Task) []*types.Node {
	reqCPU := task.ServiceConfig.Resources.CPUMilliCores
	reqMem := task.ServiceConfig.Resources.MemoryBytes

	// + overcommit
	maxCPU := int64(float64(reqCPU) * s.config.ResourceOvercommit)
	maxMem := int64(float64(reqMem) * s.config.ResourceOvercommit)

	result := make([]*types.Node, 0, len(nodes))
	for _, node := range nodes {
		node.Mu.RLock()
		availableCPU := node.Resources.CPU - node.UsedCPU
		availableMem := node.Resources.Memory - node.UsedMemory
		node.Mu.RUnlock()

		if availableCPU >= maxCPU && availableMem >= maxMem {
			result = append(result, node)
		}
	}
	return result
}

// selectRandom -> choose random Nodes
func (s *SimpleScheduler) selectRandom(nodes []*types.Node) (*types.Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no nodes to select from")
	}
	return nodes[rand.Intn(len(nodes))], nil
}

// selectRoundRobin -> round robin strategy
func (s *SimpleScheduler) selectRoundRobin(nodes []*types.Node, serviceName string) (*types.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(nodes) == 0 {
		return nil, errors.New("no nodes to select from")
	}

	// Index for service
	idx, exists := s.roundRobinIndex[serviceName]
	if !exists || idx >= len(nodes) {
		idx = 0
	}

	// Choose Node
	node := nodes[idx]

	// Update index
	s.roundRobinIndex[serviceName] = (idx + 1) % len(nodes)

	return node, nil
}

// selectBinpack -> maximum load Node (чтобы максимально уплотнить задачи)
func (s *SimpleScheduler) selectBinpack(nodes []*types.Node, task *types.Task) (*types.Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no nodes to select from")
	}

	reqCPU := task.ServiceConfig.Resources.CPUMilliCores
	reqMem := task.ServiceConfig.Resources.MemoryBytes

	// Sort by min resource
	sort.Slice(nodes, func(i, j int) bool {
		iCPULeft := nodes[i].Resources.CPU - nodes[i].UsedCPU - reqCPU
		iMemLeft := nodes[i].Resources.Memory - nodes[i].UsedMemory - reqMem
		jCPULeft := nodes[j].Resources.CPU - nodes[j].UsedCPU - reqCPU
		jMemLeft := nodes[j].Resources.Memory - nodes[j].UsedMemory - reqMem

		// By CPU -> By memory
		if iCPULeft == jCPULeft {
			return iMemLeft < jMemLeft
		}
		return iCPULeft < jCPULeft
	})

	return nodes[0], nil
}

// selectSpread -> choose minimum load Node
func (s *SimpleScheduler) selectSpread(nodes []*types.Node) (*types.Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no nodes to select from")
	}

	// Sort by Task count (меньше задач - приоритетнее)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].TaskCount < nodes[j].TaskCount
	})

	return nodes[0], nil
}

// selectLeastTasks -> min Task count
func (s *SimpleScheduler) selectLeastTasks(nodes []*types.Node) (*types.Node, error) {
	return s.selectSpread(nodes) // same logic
}

// selectLeastResource -> choose max resource Node
func (s *SimpleScheduler) selectLeastResource(nodes []*types.Node, task *types.Task) (*types.Node, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no nodes to select from")
	}

	reqCPU := task.ServiceConfig.Resources.CPUMilliCores
	reqMem := task.ServiceConfig.Resources.MemoryBytes

	// Sort by max Resource
	sort.Slice(nodes, func(i, j int) bool {
		iCPUFree := nodes[i].Resources.CPU - nodes[i].UsedCPU
		iMemFree := nodes[i].Resources.Memory - nodes[i].UsedMemory
		jCPUFree := nodes[j].Resources.CPU - nodes[j].UsedCPU
		jMemFree := nodes[j].Resources.Memory - nodes[j].UsedMemory

		// Normalize and compare
		iScore := float64(iCPUFree)/float64(reqCPU) + float64(iMemFree)/float64(reqMem)
		jScore := float64(jCPUFree)/float64(reqCPU) + float64(jMemFree)/float64(reqMem)

		return iScore > jScore
	})

	return nodes[0], nil
}

// updateNodeResources -> update resources in usage
func (s *SimpleScheduler) updateNodeResources(nodeID string, task *types.Task) {
	s.mu.RLock()
	node, exists := s.nodes[nodeID]
	s.mu.RUnlock()

	if !exists {
		return
	}

	node.Mu.Lock()
	defer node.Mu.Unlock()

	node.UsedCPU += task.ServiceConfig.Resources.CPUMilliCores
	node.UsedMemory += task.ServiceConfig.Resources.MemoryBytes
	node.TaskCount++
	node.LastSeen = time.Now()
}

// GetNodes -> Get All Nodes
func (s *SimpleScheduler) GetNodes(ctx context.Context) ([]*types.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]*types.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// RegisterNode -> New Node
func (s *SimpleScheduler) RegisterNode(ctx context.Context, node *types.Node) error {
	if node.ID == "" {
		return errors.New("node ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[node.ID]; exists {
		return fmt.Errorf("node with ID %s already exists", node.ID)
	}

	node.LastSeen = time.Now()
	node.TaskCount = 0
	node.UsedCPU = 0
	node.UsedMemory = 0
	s.nodes[node.ID] = node

	s.logger.Info("node registered", "node_id", node.ID, "hostname", node.Hostname)
	return nil
}

// UnregisterNode -> Delete Node
func (s *SimpleScheduler) UnregisterNode(ctx context.Context, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[nodeID]; !exists {
		return fmt.Errorf("node with ID %s not found", nodeID)
	}

	delete(s.nodes, nodeID)
	s.logger.Info("node unregistered", "node_id", nodeID)
	return nil
}

// UpdateNodeStatus -> Upadte Node Status
func (s *SimpleScheduler) UpdateNodeStatus(ctx context.Context, nodeID string, status types.NodeStatus) error {
	s.mu.RLock()
	node, exists := s.nodes[nodeID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node with ID %s not found", nodeID)
	}

	node.Mu.Lock()
	defer node.Mu.Unlock()

	node.Status = status
	node.LastSeen = time.Now()

	s.logger.Debug("node status updated", "node_id", nodeID, "status", status)
	return nil
}

// GetNode -> Get Node By ID
func (s *SimpleScheduler) GetNode(ctx context.Context, nodeID string) (*types.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node with ID %s not found", nodeID)
	}

	return node, nil
}

// GetNodeStats -> Get statistics by Node
func (s *SimpleScheduler) GetNodeStats(ctx context.Context, nodeID string) (*types.NodeStats, error) {
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	stats := &types.NodeStats{
		NodeID:      node.ID,
		TotalTasks:  node.TaskCount,
		CPUUsage:    float64(node.UsedCPU) / float64(node.Resources.CPU) * 100,
		MemoryUsage: float64(node.UsedMemory) / float64(node.Resources.Memory) * 100,
	}

	// Count of started Tasks
	stats.RunningTasks = node.TaskCount

	return stats, nil
}
