package types

import (
	"sync"
	"time"
)

// Types, that used in orchestration process: config structs and some node / service data

// Protocol -> typed constant with two values
type Protocol string

const (
	TCP Protocol = "tcp" // tcp protocol
	UDP Protocol = "udp" // udp protocol
)

// PortMapping -> Port host, Container Port, Protocol
type PortMapping struct {
	HostPort      int      `yaml:"host_port" json:"host_port"`           // Host port (server/computer)
	ContainerPort int      `yaml:"container_port" json:"conatiner_port"` // Container port
	Protocol      Protocol `yaml:"protocol" json:"protocol"`             // TCP / UDP
}

// ResourceRequirements -> Resources allocated to the container
type ResourceRequirements struct {
	CPUMilliCores int64  `yaml:"cpu_millicores" json:"cpu_millicores"`
	MemoryBytes   int64  `yaml:"memory_bytes" json:"memory_bytes"`
	DiskBytes     int64  `yaml:"disk_bytes" json:"disk_bytes"`
	CPUSet        string `yaml:"cpu_set" json:"cpu_set"` // Example: "0-3", "0,1"
}

// ScalePolicu -> policy for auto-scaling
type ScalePolicy struct {
	MinReplicas     int     `yaml:"min_replicas" json:"min_replicas"`         // Minimal amount of container (service) replicas
	MaxReplicas     int     `yaml:"max_replicas" json:"max_replicas"`         // Maximum amount of container (service) replicas
	TargetCPU       float64 `yaml:"target_cpu" json:"target_cpu"`             // 70.0 = 70% (for auto-scaling)
	TargetMemory    float64 `yaml:"target_memory" json:"target_memory"`       // Same as TargetCPU, but memory
	CooldownSeconds int     `yaml:"cooldown_seconds" json:"cooldown_seconds"` // TODO: what is it?
}

// HealthCheck -> Service stats checking
type HealthCheck struct {
	Type        string        `yaml:"type" json:"type"` // "http", "tcp", "command"
	HTTPPath    string        `yaml:"http_path" json:"http_path"`
	Port        int           `yaml:"port" json:"port"`
	Command     []string      `yaml:"command" json:"command"`
	Interval    time.Duration `yaml:"interval" json:"interval"`
	Timeout     time.Duration `yaml:"timeout" json:"timeout"`
	Retries     int           `yaml:"retries" json:"retries"`
	StartPeriod time.Duration `yaml:"start_period" json:"start_period"`
}

// Event -> событие
type Event struct {
	ID        string                 `json:"id"`             // ID события
	Type      string                 `json:"type"`           // Тип события: 'service_scaled, node_added, task_failed'
	Message   string                 `json:"message"`        // Сообщение
	Timestamp time.Time              `json:"timestamp"`      // Время собятия
	Data      map[string]interface{} `json:"data,omitempty"` // Дата события
}

// <---- NODE TYPES ---->

// NodeStatus представляет состояние узла
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

// ORCHESTRATOR

// OchestratorConfig -> main orchestrator configuration
type OchestratorConfig struct {
	Env         string          `yaml:"env" env-default:"local"`                    // Orchestrator enviroment
	ListenAddr  string          `yaml:"listen_addr" env-default:"localhost:8080"`   // To orchestrator API
	DataDir     string          `yaml:"data_dir" env-default:"./orchestrator-data"` // Local data of Orchestrator
	ClusterName string          `yaml:"cluster_name" env-default:"default-name"`    // Name of the Cluster
	Services    []ServiceConfig `yaml:"services"`                                   // Services for orchestration
}

// TODO: comments
type ServiceConfig struct {
	ServiceName   string        `yaml:"service_name" json:"service_name"`
	Image         string        `yaml:"image" json:"image"`
	Replicas      int           `yaml:"replicas" json:"replicas"`
	Ports         []PortMapping `yaml:"ports" json:"ports"`
	Env           []string      `yaml:"env" json:"env"`
	Command       []string      `yaml:"command" json:"command"`
	Volumes       []string      `yaml:"volumes" json:"volumes"`
	Network       string        `yaml:"network" json:"network"`
	NetworkMode   string        `yaml:"network_mode" json:"network_mode"`
	DNS           []string      `yaml:"dns" json:"dns"`
	ExtraHosts    []string      `yaml:"extra_hosts" json:"extra_hosts"`
	RestartPolicy string        `yaml:"restart_policy" json:"restart_policy"`

	Resources   ResourceRequirements `yaml:"resources"`    // Resources for service
	ScalePolicy ScalePolicy          `yaml:"scale_policy"` // Scaling policy
	HealthCheck HealthCheck          `yaml:"health_check"` // Health checking
}
