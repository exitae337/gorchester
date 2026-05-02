package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/exitae337/gorchester/internal/types"
)

type MetricsCollector struct {
	dockerClient *client.Client
	metricsStore *MetricsStore
	mu           sync.RWMutex
	stopCh       chan struct{}
}

// MetricsStore metrics history
type MetricsStore struct {
	mu      sync.RWMutex
	metrics map[string][]types.ContainerMetric // key: serviceName
	maxSize int                                // max amount of records by service
}

// Constructor
func NewMetricscollector(dockerClient *client.Client) *MetricsCollector {
	return &MetricsCollector{
		dockerClient: dockerClient,
		metricsStore: NewMetricsStore(1000),
		stopCh:       make(chan struct{}),
	}
}

func NewMetricsStore(maxSize int) *MetricsStore {
	return &MetricsStore{
		metrics: make(map[string][]types.ContainerMetric),
		maxSize: maxSize,
	}
}

// Collect metrics for container by ID
func (mc *MetricsCollector) CollectContainerMetrics(ctx context.Context, containerID string) (*types.ContainerMetric, error) {
	stats, err := mc.dockerClient.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats, containerID: %s", containerID)
	}

	var dockerStats container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&dockerStats); err != nil {
		return nil, fmt.Errorf("failed to parse JSON data from container, containerID: %s", containerID)
	}

	metrics := &types.ContainerMetric{
		ContainerID: containerID,
		Timestamp:   time.Now(),
	}

	// Cpu usage
	cpuDelta := float64(dockerStats.CPUStats.CPUUsage.TotalUsage - dockerStats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(dockerStats.CPUStats.SystemUsage - dockerStats.PreCPUStats.SystemUsage)

	if systemDelta > 0 && cpuDelta > 0 {
		cpuCores := float64(len(dockerStats.CPUStats.CPUUsage.PercpuUsage))
		metrics.CPUPercent = (cpuDelta / systemDelta) * cpuCores * 100.0
	}

	// Memory usage
	if dockerStats.MemoryStats.Limit > 0 {
		metrics.MemoryUsage = int64(dockerStats.MemoryStats.Usage)
		metrics.MemoryLimit = int64(dockerStats.MemoryStats.Limit)
		metrics.MemoryPercent = float64(dockerStats.MemoryStats.Usage) / float64(dockerStats.MemoryStats.Limit) * 100.0
	}

	// Network metrics
	for _, net := range dockerStats.Networks {
		metrics.NetworkRx += int64(net.RxBytes)
		metrics.NetworkTx += int64(net.TxBytes)
	}

	return metrics, nil
}

// Store metrics to store (in-memory store)
func (ms *MetricsStore) StoreMetrics(metrics *types.ContainerMetric) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	serviceMetrics, exists := ms.metrics[metrics.ServiceName]
	if !exists {
		serviceMetrics = make([]types.ContainerMetric, 0, ms.maxSize)
	}

	serviceMetrics = append(serviceMetrics, *metrics)

	if len(serviceMetrics) > ms.maxSize {
		serviceMetrics = serviceMetrics[len(serviceMetrics)-ms.maxSize:]
	}

	ms.metrics[metrics.ServiceName] = serviceMetrics
}

// Get service metrics
func (ms *MetricsStore) GetServiceMetrics(serviceName string, duration time.Duration) (*types.ServiceMetrics, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	serviceMetrics, exists := ms.metrics[serviceName]
	if !exists {
		return nil, fmt.Errorf("no such metrics fo service: %s", serviceName)
	}

	cutoff := time.Now().Add(-duration)
	var totalCPU, totalMem float64

	count := 0

	for _, m := range serviceMetrics {
		if m.Timestamp.After(cutoff) {
			totalCPU += m.CPUPercent
			totalMem += m.MemoryPercent
			count++
		}
	}

	if count == 0 {
		return nil, fmt.Errorf("no metrics in the specified period")
	}

	return &types.ServiceMetrics{
		ServiceName:      serviceName,
		AvgCPUPercent:    totalCPU / float64(count),
		AvgMemoryPercent: totalMem / float64(count),
		Timestamp:        time.Now(),
	}, nil
}

// Get metrics History
func (ms *MetricsStore) GetMetricsHistory(serviceName string) []types.ContainerMetric {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if metrics, exists := ms.metrics[serviceName]; exists {
		result := make([]types.ContainerMetric, len(metrics))
		copy(result, metrics)
		return result
	}

	return nil
}
