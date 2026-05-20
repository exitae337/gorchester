package types

import (
	"time"
)

// Types, that used in orchestration process: config structs and service data

// Protocol -> typed constant with two values
type Protocol string

const (
	TCP Protocol = "tcp" // tcp protocol
	UDP Protocol = "udp" // udp protocol
)

// ServiceType -> service type for adapt-planning
type ServiceType string

const (
	ServiceTypeStateless ServiceType = "stateless" // web-server, API
	ServiceTypeStateful  ServiceType = "stateful"  // data-base
	ServiceTypeBatch     ServiceType = "batch"     // period-tasks
	ServiceTypeDaemon    ServiceType = "daemon"    // system-service
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

// ScalePolicy -> policy for auto-scaling
type ScalePolicy struct {
	MinReplicas       int                      `yaml:"min_replicas" json:"min_replicas"`         // Minimal amount of container (service) replicas
	MaxReplicas       int                      `yaml:"max_replicas" json:"max_replicas"`         // Maximum amount of container (service) replicas
	TargetCPU         float64                  `yaml:"target_cpu" json:"target_cpu"`             // 70.0 = 70% (for auto-scaling)
	TargetMemory      float64                  `yaml:"target_memory" json:"target_memory"`       // Same as TargetCPU, but memory
	CooldownSeconds   int                      `yaml:"cooldown_seconds" json:"cooldown_seconds"` // TODO: what is it?
	PredictiveScaling *PredictiveScalingConfig `yaml:"predictive_scaling,omitempty" json:"predictive_scaling,omitempty"`
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

// Event -> под вопросом, Нужна ли эта структура?
type Event struct {
	ID        string                 `json:"id"`             // Event ID
	Type      string                 `json:"type"`           // Event Type: 'service_scaled, node_added, task_failed'
	Message   string                 `json:"message"`        // Message
	Timestamp time.Time              `json:"timestamp"`      // Event time
	Data      map[string]interface{} `json:"data,omitempty"` // Event date
}

// ORCHESTRATOR

// OchestratorConfig -> main orchestrator configuration
type OchestratorConfig struct {
	Env         string          `yaml:"env" env-default:"local"`                    // Orchestrator enviroment
	ListenAddr  string          `yaml:"listen_addr" env-default:"localhost:8080"`   // To orchestrator API
	DataDir     string          `yaml:"data_dir" env-default:"./orchestrator-data"` // Local data of Orchestrator
	ClusterName string          `yaml:"cluster_name" env-default:"default-name"`    // Name of the Cluster
	Services    []ServiceConfig `yaml:"services"`                                   // Services for orchestration
	Nodes       []NodeConfig    `yaml:"nodes"`                                      // Nodes from cfg
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

	ServiceType           ServiceType            `yaml:"service_type" json:"service_type"`
	SchedulingConstraints *SchedulingConstraints `yaml:"scheduling_constraints,omitempty" json:"scheduling_constraints,omitempty"`

	Resources   ResourceRequirements `yaml:"resources"`    // Resources for service
	ScalePolicy ScalePolicy          `yaml:"scale_policy"` // Scaling policy
	HealthCheck *HealthCheck         `yaml:"health_check"` // Health checking
}

// ExecConfig struct -> run in Container for HealthCheck
type ExecConfig struct {
	Cmd          []string //Commands which will be running in the container
	AttachStdOut bool     // To STD out
	AttachStdErr bool     // With Errors
}

// Scheduling data structs rules
type SchedulingConstraints struct {
	Affinity     []AffinityRule `yaml:"affinity,omitempty" json:"affinity,omitempty"`
	AntiAffinity []AffinityRule `yaml:"anti_affinity,omitempty" json:"anti_affinity,omitempty"`
}

type AffinityRule struct {
	Type     string   `yaml:"type" json:"type"`
	Operator string   `yaml:"operator" json:"operator"`
	Values   []string `yaml:"values,omitempty" json:"values,omitempty"`
}

// PredictiveScalingConfig for predictive scheduling
type PredictiveScalingConfig struct {
	Enabled          bool    `yaml:"enabled" json:"enabled"`
	LookbackWindow   int     `yaml:"lookback_window" json:"lookback_window"`     // секунд
	PredictionWindow int     `yaml:"prediction_window" json:"prediction_window"` // секунд
	CPUThreshold     float64 `yaml:"cpu_threshold" json:"cpu_threshold"`         // процент
	MemoryThreshold  float64 `yaml:"memory_threshold" json:"memory_threshold"`   // процент
}

// Container metrics
type ContainerMetric struct {
	ContainerID   string    `json:"container_id"`
	TaskID        string    `json:"task_id"`
	ServiceName   string    `json:"service_name"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsage   int64     `json:"memory_usage"`
	MemoryLimit   int64     `json:"memory_limit"`
	MemoryPercent float64   `json:"memory_percent"`
	NetworkRx     int64     `json:"network_rx"`
	NetworkTx     int64     `json:"network_tx"`
	Timestamp     time.Time `json:"timestamp"`
}

// ServiceMetrics data
type ServiceMetrics struct {
	ServiceName      string    `json:"service_name"`
	AvgCPUPercent    float64   `json:"avg_cpu_percent"`
	AvgMemoryPercent float64   `json:"avg_memory_percent"`
	TotalContainers  int       `json:"total_containers"`
	Timestamp        time.Time `json:"timestamp"`
}
