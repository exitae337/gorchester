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

// Load orchestrator configuration -> Main Loading configuration process
func MustLoad() *types.OchestratorConfig {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file for orchestrator does not exists: %s", configPath)
	}

	var config types.OchestratorConfig

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
func validateConfig(config *types.OchestratorConfig) error {
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

// LoadConfig -> for validation process
func LoadConfig(path string) (*types.OchestratorConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	var config types.OchestratorConfig
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
func applyDefaults(config *types.OchestratorConfig) {
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
