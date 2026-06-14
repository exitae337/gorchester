# Go Orchestrator

A lightweight, educational container orchestrator written in Go, developed as a Master's degree thesis project. This project aims to demystify the core concepts behind systems like Kubernetes by building a minimal yet functional orchestrator from the ground up with easy configuration.

---

## Project Overview

Modern applications are often built as a set of microservices running in containers. Orchestrators automate the deployment, scaling, and management of these containerized applications. This project is a hands-on implementation of such a system, focusing on the fundamental scheduling and cluster management algorithms.

The goal is not to build a production-ready system, but to create a functional prototype that demonstrates the core principles of container orchestration.

## Features

| Component | Description |
| :--- | :--- |
| REST API | Cluster state access, service and task management |
| Scheduler | Adaptive placement with 6 strategies, affinity/anti-affinity rules |
| Reconciliation Loop | Continuous desired vs actual state comparison, self-healing |
| Health Checks | HTTP, TCP, and command-based container health monitoring |
| Predictive Auto-scaling | Linear regression on historical metrics for proactive scaling |
| Task Store | In-memory storage with multi-index lookup, thread-safe access |
| Metrics Collection | CPU, memory, and network metrics via Docker Stats API |
| Declarative Configuration | YAML-based service definition with validation |

## Architecture

The orchestrator runs as a single binary and communicates with Docker Engine on managed nodes. All components — scheduling, health checking, metrics collection, and API — execute within the same process, eliminating the need for external databases or message brokers.

| Component | Role |
| :--- | :--- |
| Core | Reconciliation Loop, Health Check Loop, Cleanup Loop, predictive scaling |
| Scheduler | Node selection via 6 strategies, adaptive to service type |
| Docker Client | Container lifecycle management through Docker SDK |
| Task Store | In-memory state storage with indexes by container, node, service, and status |
| Metrics | Periodic CPU/memory/network collection, ring buffer storage |
| API Server | REST endpoints for services, nodes, tasks, metrics, and configuration |

## Getting Started

### Prerequisites

*   **Go 1.21+**: Make sure you have Go installed on your system.

### Installation

1.  Clone the repository:
    ```bash
    git clone https://github.com/exitae337/orchestergo.git
    cd your_repo_name
    ```

2.  Build the project:
    ```bash
    make build
    make run
    ```
    Or manually:
    ```bash
    go build -o gorchester cmd/gorchester/main.go
    ./gorchester
    ```

### Configuration File Documentation: `config.yaml`

## Overview

The `config.yaml` file is used to configure the `gorchester` orchestrator. All values have defaults but can be overridden as needed.

## Configuration Structure

### Root Level Configuration

| Field | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `env` | string | no | `"local"` | Runtime environment: `local`, `dev`, `prod` |
| `listen_addr` | string | no | `":8080"` | API server listen address |
| `data_dir` | string | no | `"./orchestrator-data"` | Data directory |
| `cluster_name` | string | no | `"default-cluster"` | Cluster identifier |
| `nodes` | array | yes | — | Compute nodes configuration |
| `services` | array | yes | — | Services to orchestrate |

## Services

Each service describes one application/microservice to be deployed.

### Service Fields

#### Basic Settings

| Field | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `service_name` | string | yes | — | Unique service identifier |
| `image` | string | yes | — | Docker image name |
| `service_type` | string | no | `"stateless"` | `stateless`, `stateful`, `batch`, `daemon` |
| `replicas` | integer | yes | — | Desired number of instances |
| `ports` | array | no | — | Port mappings |
| `command` | array | no | — | Container command override |
| `env` | array | no | — | Environment variables |
| `volumes` | array | no | — | Volume mounts |
| `network_mode` | string | no | — | Docker network mode |
| `dns` | array | no | — | DNS servers |
| `extra_hosts` | array | no | — | Extra hosts entries |
| `restart_policy` | string | no | `"no"` | `no`, `always`, `on-failure`, `unless-stopped` |
| `resources` | object | yes | — | CPU and memory limits |
| `scale_policy` | object | no | — | Auto-scaling settings |
| `health_check` | object | no | — | Health check settings |
| `scheduling_constraints` | object | no | — | Affinity and anti-affinity rules |

### Port Mapping

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `host_port` | integer | yes | Host port (0 = random assignment) |
| `container_port` | integer | yes | Container port |
| `protocol` | string | yes | `tcp` or `udp` |

### Resources

| Field | Type | Required | Minimum | Description |
| :--- | :--- | :--- | :--- | :--- |
| `cpu_millicores` | integer | yes | 5 | CPU in millicores (1000 = 1 core) |
| `memory_bytes` | integer | yes | 16 MB | Memory in bytes |

### Scale Policy

| Field | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `min_replicas` | integer | no | 1 | Minimum replica count |
| `max_replicas` | integer | no | `min_replicas` | Maximum replica count |
| `target_cpu` | float | no | 50.0 | Target CPU utilization (%) |
| `target_memory` | float | no | 50.0 | Target memory utilization (%) |
| `cooldown_seconds` | integer | no | 10 | Cooldown between scaling actions |
| `predictive_scaling` | object | no | disabled | Predictive scaling settings |

### Predictive Scaling

| Field | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | boolean | no | `false` | Enable predictive scaling |
| `lookback_window` | integer | no | 300 | History window in seconds |
| `prediction_window` | integer | no | 60 | Forecast horizon in seconds |
| `cpu_threshold` | float | no | 70.0 | CPU threshold for scale-up (%) |
| `memory_threshold` | float | no | 80.0 | Memory threshold for scale-up (%) |

### Health Check

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `type` | string | no | `http`, `tcp`, or `command` |
| `http_path` | string | for http | HTTP path |
| `port` | integer | for http/tcp | Port number |
| `command` | array | for command | Command to execute |
| `interval` | duration | no | Check interval |
| `timeout` | duration | no | Check timeout |
| `retries` | integer | no | Failures before marking unhealthy |
| `start_period` | duration | no | Grace period before first check |

### Scheduling Constraints

| Field | Type | Description |
| :--- | :--- | :--- |
| `affinity` | array | Rules preferring nodes with matching labels |
| `anti_affinity` | array | Rules excluding nodes with matching labels |

Each rule:

| Field | Type | Description |
| :--- | :--- | :--- |
| `type` | string | Label key |
| `operator` | string | `in` |
| `values` | array | Label values |

## Configuration Example

```yaml
env: "local"
listen_addr: ":8080"
cluster_name: "dev-cluster"

nodes:
  - id: "node-1"
    hostname: "worker-1.local"
    ip: "192.168.1.101"
    cpu: 4000
    memory: 17179869184
    labels:
      zone: "a"

services:
  - service_name: "web-frontend"
    image: "nginx:alpine"
    service_type: "stateless"
    replicas: 2
    ports:
      - host_port: 0
        container_port: 80
        protocol: "tcp"
    resources:
      cpu_millicores: 200
      memory_bytes: 134217728
    scale_policy:
      min_replicas: 1
      max_replicas: 6
      target_cpu: 70.0
      target_memory: 80.0
      predictive_scaling:
        enabled: true
        lookback_window: 300
        prediction_window: 60
        cpu_threshold: 75.0
        memory_threshold: 85.0
    health_check:
      type: "http"
      http_path: "/"
      port: 80
      interval: "10s"
      timeout: "5s"
      retries: 3

  - service_name: "redis-cache"
    image: "redis:alpine"
    service_type: "stateful"
    replicas: 1
    ports:
      - host_port: 0
        container_port: 6379
        protocol: "tcp"
    resources:
      cpu_millicores: 100
      memory_bytes: 67108864
    health_check:
      type: "tcp"
      port: 6379
      interval: "15s"
      timeout: "5s"
      retries: 3

  - service_name: "batch-job"
    image: "busybox:latest"
    service_type: "batch"
    replicas: 0
    command:
      - "sh"
      - "-c"
      - "echo 'Processing...' && sleep 600"
    resources:
      cpu_millicores: 100
      memory_bytes: 67108864
```

## API Reference

## API Reference

| Method | Path | Description |
| :--- | :--- | :--- |
| GET | `/api/v1/health` | Orchestrator health status |
| GET | `/api/v1/services` | List services with replica counts |
| GET | `/api/v1/services/{name}` | Service details with task list |
| GET | `/api/v1/nodes` | Node list with resource utilization |
| GET | `/api/v1/tasks` | All tasks with container IDs and status |
| GET | `/api/v1/metrics` | Current CPU and memory metrics per service |
| PUT | `/api/v1/config/strategy` | Change scheduling strategy (in progress) |

Strategy change request body:
    ```json
    {"strategy": "binpack"}
    ```

Valid strategies: random, round_robin, binpack, spread, least_tasks, least_resource.

### Scheduling

| Strategy | Behavior |
| :--- | :--- |
| `random` | Uniform random node from feasible set |
| `round_robin` | Per-service cyclic iteration |
| `binpack` | Fill densest node first, minimize active nodes |
| `spread` | Fill emptiest node first, maximize failure tolerance |
| `least_tasks` | Node with minimum task count |
| `least_resource` | Node with maximum free resources |

Adaptive selection by service type:

| Service Type | Strategy |
| :--- | :--- |
| `stateless` | spread |
| `stateful` | least_resource |
| `batch` | binpack |
| `daemon` | all feasible nodes |

## Validation

The orchestrator automatically validates the configuration.

To validate configuration without starting the orchestrator:
```bash
go run cmd/validator/main.go
```

## Contributing
As this is a Master's thesis project, the primary development is done by me. However, constructive feedback, suggestions, and discussions are highly welcome! Please feel free to open an issue to start a conversation.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Academic Context
This project is developed as a core component of my Master's thesis in "Computer Science" at SBMPEI. The focus is on researching and implementing scheduling algorithms and distributed systems principles.
