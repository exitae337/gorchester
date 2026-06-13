// Package types. Типы данных для работы с логическими узлами.
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

// NodeStats
type NodeStats struct {
	NodeID       string
	TotalTasks   int
	RunningTasks int
	CPUUsage     float64
	MemoryUsage  float64
	Uptime       time.Duration
}

// NodeConfig
type NodeConfig struct {
	ID       string            `yaml:"id" json:"id"`
	Hostname string            `yaml:"hostname" json:"hostname"`
	IP       string            `yaml:"ip" json:"ip"`
	CPU      int64             `yaml:"cpu" json:"cpu"`
	Memory   int64             `yaml:"memory" json:"memory"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}
