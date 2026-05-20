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

	// Применяем scheduling constraints если такие есть
	if task.ServiceConfig.SchedulingConstraints != nil {
		nodes = s.applyConstraints(nodes, task.ServiceConfig.SchedulingConstraints)
		if len(nodes) == 0 {
			return "", fmt.Errorf("no nodes sutisfy scheduling constraints")
		}
	}

	// Выбор ноды согласно установкам
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
		// По умолчанию используем стратегию из конфигурации
		return s.SelectNode(ctx, task, nodes)
	}
}

// ApplyConstraints - фильтрует ноды согласно affinity/anti-affinity правилам
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

// SelectSpreadWithAffinity - распределение с учетом affinity и равномерности
func (s *SimpleScheduler) selectSpreadWithAffinity(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no avaliable nodes for stateless service")
	}

	// Сортировка по TaskCount
	sort.Slice(nodes, func(i, j int) bool {
		// Приоритет имеют ноды в нужной зоне
		iPreferred := s.isNodePreferred(nodes[i], task)
		jPreferred := s.isNodePreferred(nodes[j], task)

		if iPreferred != jPreferred {
			return iPreferred
		}

		return nodes[i].TaskCount < nodes[j].TaskCount
	})

	return nodes[0].ID, nil
}

// SelectStatefulNode - выбираем ноду для сервисов с состоянием (с anti-affinity)
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

	// Приоритет нодам в неиспользуемых зонах
	sort.Slice(nodes, func(i, j int) bool {
		iZone, _ := nodes[i].Labels["zone"]
		jZone, _ := nodes[j].Labels["zone"]

		iUsed := usedZones[iZone]
		jUsed := usedZones[jZone]

		if iUsed != jUsed {
			return !iUsed // в приоритете неиспользумые зоны
		}

		// Выбираем менее загруженную если оба в одинаковом состоянии
		return nodes[i].TaskCount < nodes[j].TaskCount
	})

	return nodes[0].ID, nil
}

// SelectBinpackForBatch - оптимизация для batch-задач
func (s *SimpleScheduler) selectBinpackForBatch(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes avaliable for batch-job")
	}

	// Для batch-задачи выбираем ноду с максимальной загрузкой, но с достаточными ресурсами
	sort.Slice(nodes, func(i, j int) bool {
		iCPUUtil := float64(nodes[i].UsedCPU) / float64(nodes[i].Resources.CPU)
		jCPUUtil := float64(nodes[j].UsedCPU) / float64(nodes[j].Resources.CPU)

		return iCPUUtil > jCPUUtil // более загруженные в приоритете
	})

	return nodes[0].ID, nil
}

// SelectDaemon - для сервисов-демононов - по одному на ноду
func (s *SimpleScheduler) selectDaemonNode(nodes []*types.Node, task *types.Task) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes avaliable for daemon service")
	}

	// Ищем подходящую ноду для деммон-сервиса
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

// Вспомогательные функции
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
