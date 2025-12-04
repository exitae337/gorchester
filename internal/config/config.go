package config

import (
	"fmt"
	"log"
	"os"
	"strings"

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

type ServiceConfig struct {
	ServiceName string               `yaml:"service_name" env-required:"true"` // Service name
	Image       string               `yaml:"image" env-required:"true"`        // Docker-image name
	Replicas    int                  `yaml:"replicas" env-default:"1"`         // Count of service instances
	PortMapping []types.PortMapping  `yaml:"ports"`                            // Port mapping
	Resources   types.ResourceLimits `yaml:"resources"`                        // Resources for service
	ScalePolicy types.ScalePolicy    `yaml:"scale_policy"`                     // Scaling policy
	HealthCheck types.HealthCheck    `yaml:"health_check"`                     // Health checking
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

	if err := validateConfig(&config); err != nil {
		log.Fatalf("error occurred while validating configuration file:\n%s", err)
	}

	return &config
}

func validateConfig(config *OchestratorConfig) error {
	// Check service amd image names
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
		if service.Resources.CPUMillicores < MinMilliCores {
			errorString.WriteString(fmt.Sprintf("service[%d] amount of resources (millicores) can't be less than 5\n", i))
		}
		if service.Resources.MemoryBytes < MinMemoryBytes {
			errorString.WriteString(fmt.Sprintf("service[%d] amount of resources (memory in bytes) can't be less than 16 Mb\n", i))
		}
		// Scaling policy check
		if service.ScalePolicy.MinReplicas <= 0 {
			errorString.WriteString(fmt.Sprintf("service[%d] scale policy min_replicas can't be less or equal to zero\n", i))
		}
		if service.ScalePolicy.TargetCPU < 5.0 {
			errorString.WriteString(fmt.Sprintf("service[%d] scale policy target_cpu can't be less than 5.0 (5 percent)\n", i))
		}
		if service.ScalePolicy.TargetMemory < 10.0 {
			errorString.WriteString(fmt.Sprintf("service[%d] scale policy target_memory can't be less than 10.0 (10 percent)\n", i))
		}
		// "Health check" checking
		if service.HealthCheck.Interval < 1 {
			errorString.WriteString(fmt.Sprintf("service[%d] health_check interval can't be less than 1 (1 second)\n", i))
		}
		if service.HealthCheck.Retries < 0 {
			errorString.WriteString(fmt.Sprintf("service[%d] health_check retries can't be less than 0\n", i))
		}
		if service.HealthCheck.Timeout < 0 {
			errorString.WriteString(fmt.Sprintf("service[%d] health_check timeout can't be less than 0\n", i))
		}
	}
	if errorString.String() == "" {
		return nil
	} else {
		return fmt.Errorf("%s", errorString.String())
	}
}
