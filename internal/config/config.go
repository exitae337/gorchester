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
	var errorString strings.Builder

	for i, service := range config.Services {
		prefix := fmt.Sprintf("service[%d]", i)

		// Service checks
		if service.ServiceName == "" {
			errorString.WriteString(fmt.Sprintf("%s name can't be empty: required\n", prefix))
		}
		if service.Image == "" {
			errorString.WriteString(fmt.Sprintf("%s image name can't be empty: required\n", prefix))
		}

		// Replicas check (разрешён 0 для batch-сервисов)
		if service.Replicas < 0 {
			errorString.WriteString(fmt.Sprintf("%s amount of replicas can't be negative\n", prefix))
		}

		// Service type validation
		if service.ServiceType != "" {
			switch service.ServiceType {
			case types.ServiceTypeStateless, types.ServiceTypeStateful,
				types.ServiceTypeBatch, types.ServiceTypeDaemon:
				// valid
			default:
				errorString.WriteString(fmt.Sprintf(
					"%s service_type must be one of: stateless, stateful, batch, daemon\n", prefix))
			}
		}

		// Resources check
		if service.Resources.CPUMilliCores < MinMilliCores {
			errorString.WriteString(fmt.Sprintf(
				"%s amount of resources (millicores) can't be less than %d\n", prefix, MinMilliCores))
		}
		if service.Resources.MemoryBytes < MinMemoryBytes {
			errorString.WriteString(fmt.Sprintf(
				"%s amount of resources (memory in bytes) can't be less than %d bytes\n", prefix, MinMemoryBytes))
		}

		// Scaling policy check
		if service.ScalePolicy.MinReplicas < 0 {
			errorString.WriteString(fmt.Sprintf(
				"%s scale policy min_replicas can't be less than 0\n", prefix))
		}
		if service.ScalePolicy.MaxReplicas > 0 && service.ScalePolicy.MaxReplicas < service.ScalePolicy.MinReplicas {
			errorString.WriteString(fmt.Sprintf(
				"%s scale policy max_replicas cannot be less than min_replicas\n", prefix))
		}

		// Predictive scaling validation
		if service.ScalePolicy.PredictiveScaling != nil && service.ScalePolicy.PredictiveScaling.Enabled {
			ps := service.ScalePolicy.PredictiveScaling
			if ps.CPUThreshold <= 0 || ps.CPUThreshold > 100 {
				errorString.WriteString(fmt.Sprintf(
					"%s predictive_scaling cpu_threshold must be between 1 and 100\n", prefix))
			}
			if ps.MemoryThreshold <= 0 || ps.MemoryThreshold > 100 {
				errorString.WriteString(fmt.Sprintf(
					"%s predictive_scaling memory_threshold must be between 1 and 100\n", prefix))
			}
			if ps.LookbackWindow < 60 {
				errorString.WriteString(fmt.Sprintf(
					"%s predictive_scaling lookback_window must be at least 60 seconds\n", prefix))
			}
			if ps.PredictionWindow < 10 {
				errorString.WriteString(fmt.Sprintf(
					"%s predictive_scaling prediction_window must be at least 10 seconds\n", prefix))
			}
		}

		// Scheduling constraints validation
		if service.SchedulingConstraints != nil {
			for j, rule := range service.SchedulingConstraints.Affinity {
				if rule.Type == "" {
					errorString.WriteString(fmt.Sprintf(
						"%s scheduling_constraints.affinity[%d] type is required\n", prefix, j))
				}
				if rule.Operator == "" {
					errorString.WriteString(fmt.Sprintf(
						"%s scheduling_constraints.affinity[%d] operator is required\n", prefix, j))
				}
			}
			for j, rule := range service.SchedulingConstraints.AntiAffinity {
				if rule.Type == "" {
					errorString.WriteString(fmt.Sprintf(
						"%s scheduling_constraints.anti_affinity[%d] type is required\n", prefix, j))
				}
				if rule.Operator == "" {
					errorString.WriteString(fmt.Sprintf(
						"%s scheduling_constraints.anti_affinity[%d] operator is required\n", prefix, j))
				}
			}
		}

		// Health check validation
		if service.HealthCheck != nil && service.HealthCheck.Type != "" {
			if service.HealthCheck.Interval < time.Second {
				errorString.WriteString(fmt.Sprintf(
					"%s health_check interval can't be less than 1 second\n", prefix))
			}
			if service.HealthCheck.Retries < 0 {
				errorString.WriteString(fmt.Sprintf(
					"%s health_check retries can't be less than 0\n", prefix))
			}
			if service.HealthCheck.Timeout < 0 {
				errorString.WriteString(fmt.Sprintf(
					"%s health_check timeout can't be less than 0\n", prefix))
			}
			switch service.HealthCheck.Type {
			case "http":
				if service.HealthCheck.Port <= 0 {
					errorString.WriteString(fmt.Sprintf(
						"%s health_check port is required for http type\n", prefix))
				}
				if service.HealthCheck.HTTPPath == "" {
					errorString.WriteString(fmt.Sprintf(
						"%s health_check http_path is required for http type\n", prefix))
				}
			case "tcp":
				if service.HealthCheck.Port <= 0 {
					errorString.WriteString(fmt.Sprintf(
						"%s health_check port is required for tcp type\n", prefix))
				}
			case "command":
				if len(service.HealthCheck.Command) == 0 {
					errorString.WriteString(fmt.Sprintf(
						"%s health_check command is required for command type\n", prefix))
				}
			default:
				errorString.WriteString(fmt.Sprintf(
					"%s health_check type must be one of: http, tcp, command\n", prefix))
			}
		}
	}

	if errorString.String() == "" {
		return nil
	}
	return fmt.Errorf("%s", errorString.String())
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
		applyServiceDefaults(&config.Services[i])
		applyScalePolicyDefaults(&config.Services[i].ScalePolicy)
		applyHealthCheckDefaults(config.Services[i].HealthCheck)
		applyPredictiveScalingDefaults(config.Services[i].ScalePolicy.PredictiveScaling)
	}
}

// Новые значения по умолчанию для сервиса
func applyServiceDefaults(svc *types.ServiceConfig) {
	// Тип сервиса по умолчанию
	if svc.ServiceType == "" {
		svc.ServiceType = types.ServiceTypeStateless
	}

	// Если нет scheduling constraints, создаём пустые
	if svc.SchedulingConstraints == nil {
		svc.SchedulingConstraints = &types.SchedulingConstraints{}
	}
}

// ScalePolicy default values
func applyScalePolicyDefaults(sp *types.ScalePolicy) {
	if sp.MinReplicas < 0 {
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

	// Инициализируем PredictiveScaling если nil
	if sp.PredictiveScaling == nil {
		sp.PredictiveScaling = &types.PredictiveScalingConfig{
			Enabled: false,
		}
	}
}

// PredictiveScaling default values
func applyPredictiveScalingDefaults(ps *types.PredictiveScalingConfig) {
	if ps == nil {
		return
	}
	if ps.LookbackWindow == 0 {
		ps.LookbackWindow = 300 // 5 минут
	}
	if ps.PredictionWindow == 0 {
		ps.PredictionWindow = 60 // 1 минута
	}
	if ps.CPUThreshold == 0 {
		ps.CPUThreshold = 70.0
	}
	if ps.MemoryThreshold == 0 {
		ps.MemoryThreshold = 80.0
	}
}

// HealthCheck default values
func applyHealthCheckDefaults(hc *types.HealthCheck) {
	if hc == nil {
		return
	}
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
