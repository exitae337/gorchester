package types

import (
	"sync"
	"time"
)

// <---- NODE TYPES ---->

// NodeStatus -> Node Current Status
type NodeStatus string

const (
	// NodeStatusReady - ready for Tasks
	NodeStatusReady NodeStatus = "ready"
	// NodeStatusNotReady - not ready for Tasks ()connection problems
	NodeStatusNotReady NodeStatus = "not_ready"
	// NodeStatusDraining - down
	NodeStatusDraining NodeStatus = "draining"
)

// NodeResources struct -> Resources
type NodeResources struct {
	CPU    int64 // in millicores
	Memory int64 // in bytes
}

// Node struct -> node in cluster
type Node struct {
	ID        string            `json:"id"`
	Hostname  string            `json:"hostname"`
	IP        string            `json:"ip"`
	Status    NodeStatus        `json:"status"`
	Labels    map[string]string `json:"labels"`
	Resources *NodeResources    `json:"resources"`

	// resiurces -> dynamic changes
	UsedCPU    int64 `json:"used_cpu"`
	UsedMemory int64 `json:"used_memory"`

	// Count of tasks
	TaskCount int `json:"task_count"`

	// Heartbeat -> last
	LastSeen time.Time `json:"last_seen"`

	Mu sync.RWMutex `json:"-"`
}

// NodeStats представляет статистику по узлу
type NodeStats struct {
	NodeID       string
	TotalTasks   int
	RunningTasks int
	CPUUsage     float64 // процент использования
	MemoryUsage  float64 // процент использования
	Uptime       time.Duration
}
