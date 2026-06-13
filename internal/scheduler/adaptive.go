// Package scheduler. adaptive.go -> файл описывающий функции для
// реализации адаптивного механизма масштабирования.
package scheduler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/exitae337/gorchester/internal/types"
)

func (s *SimpleScheduler) SelectNodeAdaptive(ctx context.Context, task *types.Task, nodes []*types.Node) (string, error) {
	if task.ServiceConfig == nil {
		return s.SelectNode(ctx, task, nodes)
	}

	if task.ServiceConfig.SchedulingConstraints != nil {
		nodes = s.applyConstraints(nodes, task.ServiceConfig.SchedulingConstraints)
		if len(nodes) == 0 {
			return "", fmt.Errorf("no nodes sutisfy scheduling constraints")
		}
	}

	switch task.ServiceConfig.ServiceType {
	case types.ServiceTypeStateless:
		return s.selectSpreadWithAffinity(nodes, task)
	case types.ServiceTypeStateful:
		return s.selectStatefulNode(nodes, task)
	case types.ServiceTypeBatch:
		return s.selectBinpackForBatch(nodes, task)
	case types.ServiceTypeDaemon:
		return s.selectDaemonNode(nodes, task)
	default:
		return s.SelectNode(ctx, task, nodes)
	}
}

// ApplyConstraints -> filter nodes by affinity / anti-affinity rules
func (s *SimpleScheduler) applyConstraints(nodes []*types.Node, constraints *types.SchedulingConstraints) []*types.Node {
	if constraints == nil {
		return nodes
	}

	result := make([]*types.Node, 0, len(nodes))

	for _, node := range nodes {
		if s.nodeMatchesAffinity(node, constraints.Affinity) &&
			s.nodeMatchesAntiAffinity(node, constraints.AntiAffinity) {
			result = append(result, node)
		}
	}

	return result
}

func (s *SimpleScheduler) nodeMatchesAffinity(node *types.Node, rules []types.AffinityRule) bool {
	if len(rules) == 0 {
		return true
	}

	for _, rule := range rules {
		switch rule.Type {
		case "zone":
			if zone, exists := node.Labels["zone"]; exists {
				if rule.Operator == "in" && !contains(rule.Values, zone) {
					return false
				}
			}
		case "region":
			if region, exists := node.Labels["region"]; exists {
				if rule.Operator == "in" && !contains(rule.Values, region) {
					return false
				}
			}
		}
	}
	return true
}

func (s *SimpleScheduler) nodeMatchesAntiAffinity(node *types.Node, rules []types.AffinityRule) bool {
	if len(rules) == 0 {
		return true
	}

	for _, rule := range rules {
		switch rule.Type {
		case "zone":
			if zone, exists := node.Labels["zone"]; exists {
				if rule.Operator == "in" && contains(rule.Values, zone) {
					return false
				}
			}
		}
	}
	return true
}

// SelectSpreadWithAffinity -> distribution taking into account affinity and uniformity
func (s *SimpleScheduler) selectSpreadWithAffinity(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no avaliable nodes for stateless service")
	}

	sort.Slice(nodes, func(i, j int) bool {
		iPreferred := s.isNodePreferred(nodes[i], task)
		jPreferred := s.isNodePreferred(nodes[j], task)

		if iPreferred != jPreferred {
			return iPreferred
		}

		return nodes[i].TaskCount < nodes[j].TaskCount
	})

	return nodes[0].ID, nil
}

// SelectStatefulNode
func (s *SimpleScheduler) selectStatefulNode(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes avaliable for stateful service")
	}

	// Stateful сервисы должны быть распределены по разным нодам
	usedZones := make(map[string]bool)
	s.mu.RLock()
	for _, node := range s.nodes {
		for _, t := range s.getNodeTasks(node.ID) {
			if t.ServiceName == task.ServiceName && t.ID != task.ID {
				if zone, exists := node.Labels["zone"]; exists {
					usedZones[zone] = true
				}
			}
		}
	}
	s.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		iZone, _ := nodes[i].Labels["zone"]
		jZone, _ := nodes[j].Labels["zone"]

		iUsed := usedZones[iZone]
		jUsed := usedZones[jZone]

		if iUsed != jUsed {
			return !iUsed
		}

		return nodes[i].TaskCount < nodes[j].TaskCount
	})

	return nodes[0].ID, nil
}

// SelectBinpackForBatch
func (s *SimpleScheduler) selectBinpackForBatch(nodes []*types.Node, _ *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes avaliable for batch-job")
	}

	sort.Slice(nodes, func(i, j int) bool {
		iCPUUtil := float64(nodes[i].UsedCPU) / float64(nodes[i].Resources.CPU)
		jCPUUtil := float64(nodes[j].UsedCPU) / float64(nodes[j].Resources.CPU)

		return iCPUUtil > jCPUUtil
	})

	return nodes[0].ID, nil
}

// SelectDaemon
func (s *SimpleScheduler) selectDaemonNode(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes avaliable for daemon service")
	}

	for _, node := range nodes {
		hasDaemon := false
		s.mu.RLock()
		for _, t := range s.getNodeTasks(node.ID) {
			if t.ServiceName == task.ServiceName && t.Status == types.TaskStatusRunning {
				hasDaemon = true
				break
			}
		}
		s.mu.RUnlock()

		if !hasDaemon {
			return node.ID, nil
		}
	}

	return "", fmt.Errorf("daemon is already running on all nodes")
}

// ========= HELPERS =========

func (s *SimpleScheduler) isNodePreferred(node *types.Node, task *types.Task) bool {
	if task.ServiceConfig.SchedulingConstraints == nil {
		return true
	}

	for _, rule := range task.ServiceConfig.SchedulingConstraints.Affinity {
		if rule.Type == "zone" {
			if zone, exists := node.Labels["zone"]; exists {
				if contains(rule.Values, zone) {
					return true
				}
			}
		}
	}
	return false
}

func (s *SimpleScheduler) getNodeTasks(nodeID string) []*types.Task {
	if s.taskStore == nil {
		return []*types.Task{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tasks, err := s.taskStore.ListByNodeID(ctx, nodeID)
	if err != nil {
		s.logger.Debug("failed to get node tasks",
			"node_id", nodeID,
			"error", err)
		return []*types.Task{}
	}

	return tasks
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
