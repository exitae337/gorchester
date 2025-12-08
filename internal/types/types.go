package types

import "time"

// Types, that used in orchestration process: config structs and some node / service data

// Protocol -> typed constant with two values
type Protocol string

const (
	TCP Protocol = "tcp" // tcp protocol
	UDP Protocol = "udp" // udp protocol
)

// PortMapping -> Port host, Container Port, Protocol
type PortMapping struct {
	HostPort      int      `yaml:"host_port" env-default:"80"`      // Host port
	ContainerPort int      `yaml:"container_port" env-default:"80"` // Container port
	Protocol      Protocol `yaml:"protocol" env-default:"tcp"`      // TCP / UDP
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
	MinReplicas     int     `yaml:"min_replicas" env-default:"1"`      // Minimal amount of container (service) replicas
	MaxReplicas     int     `yaml:"max_replicas" env-default:"3"`      // Maximum amount of container (service) replicas
	TargetCPU       float64 `yaml:"target_cpu" env-default:"50.0"`     // 70.0 = 70% (for auto-scaling)
	TargetMemory    float64 `yaml:"target_memory" env-default:"50.0"`  // Same as TargetCPU, but memory
	CooldownSeconds int     `yaml:"cooldown_seconds" env-default:"10"` // TODO: what is it?
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

// Node -> node type
type Node struct {
	ID       string            `json:"id"`       // Node ID
	Address  string            `json:"address"`  // Node address
	Capacity NodeCapacity      `json:"capacity"` // Capacity (CPUs and memory)
	Usage    NodeUsage         `json:"usage"`    // Usage (current usage)
	Labels   map[string]string `json:"labels"`   // Headers
	Status   string            `json:"status"`   // "ready", "draining", "down"
}

// NodeCapacity
type NodeCapacity struct {
	CPU    int64 `json:"cpu"`    // in CPU millicores
	Memory int64 `json:"memory"` // in bytes
}

// NodeUsage
type NodeUsage struct {
	CPU    float64 `json:"cpu_usage"`
	Memory int64   `json:"memory_usage"`
}

// Task -> структура для задачи
type Task struct {
	ID          string            `json:"id"`          // ID задачи
	ServiceID   string            `json:"service_id"`  // ID сервиса
	NodeID      string            `json:"node_id"`     // ID ноды
	Image       string            `json:"image"`       // Имя image
	Status      string            `json:"task_status"` // Статус задачи: 'running, stopped, failed'
	Ports       []PortMapping     `json:"ports"`
	Environment map[string]string `json:"environment"`
}

// Event -> событие
type Event struct {
	ID        string                 `json:"id"`             // ID события
	Type      string                 `json:"type"`           // Тип события: 'service_scaled, node_added, task_failed'
	Message   string                 `json:"message"`        // Сообщение
	Timestamp time.Time              `json:"timestamp"`      // Время собятия
	Data      map[string]interface{} `json:"data,omitempty"` // Дата события
}
