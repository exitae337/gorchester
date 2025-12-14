package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/exitae337/gorchester/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "config/config.yaml", "Path to configuration file")
	validateOnly := flag.Bool("validate-only", false, "Only validate, don't print config")
	outputFormat := flag.String("format", "text", "Output format: text, json, yaml")
	flag.Parse()

	// Load configuration using the new LoadConfig function
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("❌ Validation failed: %v", err)
	}

	fmt.Printf("✅ Configuration validation successful!\n")
	fmt.Printf("   File: %s\n", *configPath)
	fmt.Printf("   Environment: %s\n", cfg.Env)
	fmt.Printf("   Services: %d\n", len(cfg.Services))

	if !*validateOnly {
		printConfigDetails(cfg, *outputFormat)
	}
}

func printConfigDetails(cfg *config.OchestratorConfig, format string) {
	switch format {
	case "json":
		printConfigJSON(cfg)
	case "yaml":
		printConfigYAML(cfg)
	default:
		printConfigText(cfg)
	}
}

func printConfigText(cfg *config.OchestratorConfig) {
	fmt.Println("\n📋 Configuration Details:")
	fmt.Printf("Environment: %s\n", cfg.Env)
	fmt.Printf("Listen Address: %s\n", cfg.ListenAddr)
	fmt.Printf("Data Directory: %s\n", cfg.DataDir)
	fmt.Printf("Cluster Name: %s\n", cfg.ClusterName)

	fmt.Println("\n🚀 Services:")
	for i, service := range cfg.Services {
		fmt.Printf("\n  [%d] %s\n", i+1, service.ServiceName)
		fmt.Printf("      Image: %s\n", service.Image)
		fmt.Printf("      Replicas: %d\n", service.Replicas)
		fmt.Printf("      CPU: %dm (%0.2f cores)\n",
			service.Resources.CPUMilliCores,
			float64(service.Resources.CPUMilliCores)/1000)
		fmt.Printf("      Memory: %d MB\n",
			service.Resources.MemoryBytes/(1024*1024))

		// Ports
		if len(service.Ports) > 0 {
			fmt.Printf("      Ports:\n")
			for _, port := range service.Ports {
				fmt.Printf("        %d:%d/%s\n",
					port.HostPort, port.ContainerPort, port.Protocol)
			}
		}

		// Scale Policy
		if service.ScalePolicy.MinReplicas > 0 {
			fmt.Printf("      Scale Policy:\n")
			fmt.Printf("        Min Replicas: %d\n", service.ScalePolicy.MinReplicas)
			fmt.Printf("        Max Replicas: %d\n", service.ScalePolicy.MaxReplicas)
			fmt.Printf("        Target CPU: %.1f%%\n", service.ScalePolicy.TargetCPU)
			fmt.Printf("        Target Memory: %.1f%%\n", service.ScalePolicy.TargetMemory)
			fmt.Printf("        Cooldown: %d seconds\n", service.ScalePolicy.CooldownSeconds)
		}

		// Health Check
		if service.HealthCheck.Type != "" {
			fmt.Printf("      Health Check:\n")
			fmt.Printf("        Type: %s\n", service.HealthCheck.Type)
			if service.HealthCheck.Type == "http" {
				fmt.Printf("        Path: %s\n", service.HealthCheck.HTTPPath)
			}
			if service.HealthCheck.Port > 0 {
				fmt.Printf("        Port: %d\n", service.HealthCheck.Port)
			}
			if len(service.HealthCheck.Command) > 0 {
				fmt.Printf("        Command: %v\n", service.HealthCheck.Command)
			}
			fmt.Printf("        Interval: %v\n", service.HealthCheck.Interval)
			fmt.Printf("        Timeout: %v\n", service.HealthCheck.Timeout)
			fmt.Printf("        Retries: %d\n", service.HealthCheck.Retries)
		}

		// Environment Variables
		if len(service.Env) > 0 {
			fmt.Printf("      Environment Variables: %d\n", len(service.Env))
		}

		// Volumes
		if len(service.Volumes) > 0 {
			fmt.Printf("      Volumes: %d\n", len(service.Volumes))
		}
	}
}

func printConfigJSON(cfg *config.OchestratorConfig) {
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Printf("Error generating JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonData))
}

func printConfigYAML(cfg *config.OchestratorConfig) {
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Printf("Error generating YAML: %v\n", err)
		return
	}
	fmt.Println(string(yamlData))
}
