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

func (mc *MetricsCollector) CollectContainerMetrics(ctx context.Context, containerID string) (*types.ContainerMetric, error) {
	// Используем стриминговый режим с таймаутом 5 секунд
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	statsCh, err := mc.dockerClient.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats stream, containerID: %s: %w", containerID, err)
	}
	defer statsCh.Body.Close()

	decoder := json.NewDecoder(statsCh.Body)

	// Читаем первый кадр
	var preStats container.StatsResponse
	if err := decoder.Decode(&preStats); err != nil {
		return nil, fmt.Errorf("failed to decode first stats frame, containerID: %s: %w", containerID, err)
	}

	// Ждём второй кадр с таймаутом 3 секунды
	var curStats container.StatsResponse
	decodeDone := make(chan error, 1)
	go func() {
		decodeDone <- decoder.Decode(&curStats)
	}()

	select {
	case <-ctx.Done():
		// Контекст отменён — используем первый кадр как текущий
		curStats = preStats
	case err := <-decodeDone:
		if err != nil {
			// Ошибка декодирования — используем первый кадр
			curStats = preStats
		}
	}

	metrics := &types.ContainerMetric{
		ContainerID: containerID,
		Timestamp:   time.Now(),
	}

	// CPU usage (разница между двумя кадрами)
	cpuDelta := float64(curStats.CPUStats.CPUUsage.TotalUsage - preStats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(curStats.CPUStats.SystemUsage - preStats.PreCPUStats.SystemUsage)

	if systemDelta > 0 && cpuDelta > 0 {
		cpuCores := float64(len(curStats.CPUStats.CPUUsage.PercpuUsage))
		if cpuCores == 0 {
			cpuCores = 1
		}
		metrics.CPUPercent = (cpuDelta / systemDelta) * cpuCores * 100.0
	}

	// Memory usage
	if curStats.MemoryStats.Limit > 0 {
		metrics.MemoryUsage = int64(curStats.MemoryStats.Usage)
		metrics.MemoryLimit = int64(curStats.MemoryStats.Limit)
		metrics.MemoryPercent = float64(curStats.MemoryStats.Usage) / float64(curStats.MemoryStats.Limit) * 100.0
	}

	// Network metrics
	for _, net := range curStats.Networks {
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
