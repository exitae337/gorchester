package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/exitae337/gorchester/internal/types"
	"github.com/ilyakaznacheev/cleanenv"
)

const (
	// Min CPU in millicores
	// 1m = 0.001 CPU, but Linux can't garant less than 1m
	MinMilliCores = 5 // 0.005 CPU

	// Memory - minimum memory in bytes for starting docker container
	// PAGE_SIZE * 2
	MinMemoryBytes = 16 * 1024 * 1024 // 16 MiB
)

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
	ServiceName   string              `yaml:"service_name" json:"service_name"`
	Image         string              `yaml:"image" json:"image"`
	Replicas      int                 `yaml:"replicas" json:"replicas"`
	Ports         []types.PortMapping `yaml:"ports" json:"ports"`
	Env           []string            `yaml:"env" json:"env"`
	Command       []string            `yaml:"command" json:"command"`
	Volumes       []string            `yaml:"volumes" json:"volumes"`
	Network       string              `yaml:"network" json:"network"`
	NetworkMode   string              `yaml:"network_mode" json:"network_mode"`
	DNS           []string            `yaml:"dns" json:"dns"`
	ExtraHosts    []string            `yaml:"extra_hosts" json:"extra_hosts"`
	RestartPolicy string              `yaml:"restart_policy" json:"restart_policy"`

	Resources   types.ResourceRequirements `yaml:"resources"`    // Resources for service
	ScalePolicy types.ScalePolicy          `yaml:"scale_policy"` // Scaling policy
	HealthCheck types.HealthCheck          `yaml:"health_check"` // Health checking
}

// Load orchestrator configuration
func MustLoad() *OchestratorConfig {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file for orchestrator does not exists: %s", configPath)
	}

	var config OchestratorConfig

	err := cleanenv.ReadConfig(configPath, &config)
	if err != nil {
		log.Fatalf("failed to read configuration file: %s, %s", configPath, err)
	}

	applyDefaults(&config)

	if err := validateConfig(&config); err != nil {
		log.Fatalf("error occurred while validating configuration file:\n%s", err)
	}

	return &config
}

// Configuration validation -> config.yaml into OrchestratorConfig struct with cleanenv module
func validateConfig(config *OchestratorConfig) error {
	// Check service and image names
	var errorString strings.Builder
	for i, service := range config.Services {
		// Service checks
		if service.ServiceName == "" {
			errorString.WriteString(fmt.Sprintf("service[%d] name can't be empty: required\n", i))
		}
		if service.Image == "" {
			errorString.WriteString(fmt.Sprintf("service[%d] image name can't be empty: required\n", i))
		}
		if service.Replicas <= 0 {
			errorString.WriteString(fmt.Sprintf("service[%d] amount of replicas can't be less or equal to zero\n", i))
		}
		// Resources check
		if service.Resources.CPUMilliCores < MinMilliCores {
			errorString.WriteString(fmt.Sprintf("service[%d] amount of resources (millicores) can't be less than %d\n", i, MinMilliCores))
		}
		if service.Resources.MemoryBytes < MinMemoryBytes {
			errorString.WriteString(fmt.Sprintf("service[%d] amount of resources (memory in bytes) can't be less than %d bytes\n", i, MinMemoryBytes))
		}
		// Scaling policy check
		if service.ScalePolicy.MinReplicas <= 0 {
			errorString.WriteString(fmt.Sprintf("service[%d] scale policy min_replicas can't be less or equal to zero\n", i))
		}
		if service.ScalePolicy.MaxReplicas > 0 && service.ScalePolicy.MaxReplicas < service.ScalePolicy.MinReplicas {
			errorString.WriteString(fmt.Sprintf("service[%d] scale policy max_replicas cannot be less than min_replicas\n", i))
		}
		// Health check validation
		if service.HealthCheck.Type != "" {
			if service.HealthCheck.Interval < time.Second {
				errorString.WriteString(fmt.Sprintf("service[%d] health_check interval can't be less than 1 second\n", i))
			}
			if service.HealthCheck.Retries < 0 {
				errorString.WriteString(fmt.Sprintf("service[%d] health_check retries can't be less than 0\n", i))
			}
			if service.HealthCheck.Timeout < 0 {
				errorString.WriteString(fmt.Sprintf("service[%d] health_check timeout can't be less than 0\n", i))
			}
			switch service.HealthCheck.Type {
			case "http":
				if service.HealthCheck.Port <= 0 {
					errorString.WriteString(fmt.Sprintf("service[%d] health_check port is required for http type\n", i))
				}
				if service.HealthCheck.HTTPPath == "" {
					errorString.WriteString(fmt.Sprintf("service[%d] health_check http_path is required for http type\n", i))
				}
			case "tcp":
				if service.HealthCheck.Port <= 0 {
					errorString.WriteString(fmt.Sprintf("service[%d] health_check port is required for tcp type\n", i))
				}
			case "command":
				if len(service.HealthCheck.Command) == 0 {
					errorString.WriteString(fmt.Sprintf("service[%d] health_check command is required for command type\n", i))
				}
			default:
				errorString.WriteString(fmt.Sprintf("service[%d] health_check type must be one of: http, tcp, command\n", i))
			}
		}
	}

	if errorString.String() == "" {
		return nil
	} else {
		return fmt.Errorf("%s", errorString.String())
	}
}

// LoadConfig -> for validator
func LoadConfig(path string) (*OchestratorConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	var config OchestratorConfig
	if err := cleanenv.ReadConfig(path, &config); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	applyDefaults(&config)

	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Apply defaults funcs
func applyDefaults(config *OchestratorConfig) {
	for i := range config.Services {
		applyScalePolicyDefaults(&config.Services[i].ScalePolicy)
		applyHealthCheckDefaults(&config.Services[i].HealthCheck)
	}
}

// ScalePolicy default values
func applyScalePolicyDefaults(sp *types.ScalePolicy) {
	if sp.MinReplicas == 0 {
		sp.MinReplicas = 1
	}
	if sp.MaxReplicas == 0 {
		sp.MaxReplicas = sp.MinReplicas
	}
	if sp.TargetCPU == 0 {
		sp.TargetCPU = 50.0
	}
	if sp.TargetMemory == 0 {
		sp.TargetMemory = 50.0
	}
	if sp.CooldownSeconds == 0 {
		sp.CooldownSeconds = 10
	}
}

// HealthCheck default values
func applyHealthCheckDefaults(hc *types.HealthCheck) {
	if hc.Interval == 0 {
		hc.Interval = 30 * time.Second
	}
	if hc.Timeout == 0 {
		hc.Timeout = 10 * time.Second
	}
	if hc.Retries == 0 {
		hc.Retries = 3
	}
	if hc.StartPeriod == 0 {
		hc.StartPeriod = 0 * time.Second
	}
}
