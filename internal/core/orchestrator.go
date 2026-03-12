package core

import "time"

// Orchestrator settings
type OrchestratorSettings struct {
	ReconcileInterval   time.Duration // reconcile interval
	HealthCheckInterval time.Duration // health check interval
}

// DefaultOrchestratorConfig
func DefaultOrchestratorConfig() *OrchestratorSettings {
	return &OrchestratorSettings{
		ReconcileInterval:   30 * time.Second,
		HealthCheckInterval: 15 * time.Second,
	}
}
