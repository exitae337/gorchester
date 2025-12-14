# Go Orchestrator

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Status: WIP](https://img.shields.io/badge/Status-Active%20Development-orange.svg)](https://github.com/exitae337/orchestergo)

A lightweight, educational container orchestrator written in Go, developed as a Master's degree thesis project. This project aims to demystify the core concepts behind systems like Kubernetes by building a minimal yet functional orchestrator from the ground up with easy configuration.

> **Note:** This project is currently under active development as part of my Master's program. Features and the codebase are evolving rapidly.

---

## 🚀 Project Overview

Modern applications are often built as a set of microservices running in containers. Orchestrators automate the deployment, scaling, and management of these containerized applications. This project is a hands-on implementation of such a system, focusing on the fundamental scheduling and cluster management algorithms.

The goal is not to build a production-ready system, but to create a functional prototype that demonstrates the core principles of container orchestration.

## ✨ Planned Features / Current Status

| Component | Status | Description |
| :--- | :--- | :--- |
| **REST API** | 🔄 **In Progress** | Basic HTTP API for submitting and managing jobs. |
| **Scheduler** | ⏳ **Planned** | Core scheduling logic for assigning tasks to nodes. |
| **Node Agent** | ⏳ **Planned** | Lightweight agent to run on worker nodes. |
| **Service Discovery** | 💡 **Backlog** | Automatic discovery of nodes in the cluster. |
| **Health Checks** | 💡 **Backlog** | Monitoring container and node health. |

*(This table will be updated as the project progresses.)*

## 🏗️ Architecture (High-Level)

The system is designed with a primary master node and multiple worker nodes.

1.  **Master Node:** Hosts the central brain of the orchestrator — the REST API and the Scheduler.
2.  **Worker Nodes:** Run a lightweight agent that communicates with the master, receives tasks, and executes containers.
3.  **Client:** Users interact with the system by sending requests (e.g., to run a container) to the Master's REST API.

## 🛠️ Getting Started

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
    go build -o go-orchestrator cmd/main.go
    ```

### Configuration File Documentation: `config.yaml`

## Overview

The `config.yaml` file is used to configure the `gorchester` orchestrator. All values have defaults but can be overridden as needed.

## Configuration Structure

### Root Level Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `env` | `string` | No | `"local"` | Environment: `local`, `dev`, `prod` |
| `listen_addr` | `string` | No | `":8080"` | Address and port for the orchestrator's REST API |
| `data_dir` | `string` | No | `"./orchestrator-data"` | Directory for orchestrator data storage |
| `cluster_name` | `string` | No | `"default-cluster"` | Cluster name for identification |
| `services` | `array` | **Yes** | - | List of services to orchestrate |

## Services

Each service describes one application/microservice to be deployed.

### Service Fields

#### Basic Settings

| Field | Type | Required | Example | Description |
|-------|------|----------|---------|-------------|
| `service_name` | `string` | **Yes** | `"web-server"` | Unique service name |
| `image` | `string` | **Yes** | `"nginx:alpine"` | Docker image (from Docker Hub or local) |
| `replicas` | `integer` | **Yes** | `2` | Number of container replicas (instances) |
| `ports` | `array` | No | - | Port mappings from host to container |
| `env` | `array` | No | `["KEY=value"]` | Environment variables |
| `command` | `array` | No | `["npm", "start"]` | Startup command (overrides Dockerfile CMD) |
| `volumes` | `array` | No | `["/host:/container"]` | Volume mounts |
| `network` | `string` | No | `"gorchester-network"` | Docker network to connect to |
| `network_mode` | `string` | No | `"bridge"` | Network mode: `bridge`, `host`, `none` |
| `dns` | `array` | No | `["8.8.8.8"]` | DNS servers |
| `extra_hosts` | `array` | No | `["host.docker.internal:host-gateway"]` | Additional `/etc/hosts` entries |
| `restart_policy` | `string` | No | `"always"` | Restart policy: `no`, `always`, `on-failure`, `unless-stopped` |

#### Port Mapping (PortMapping)

Each element in the `ports` array has the following structure:

```yaml
ports:
  - host_port: 8080      # Port on the host
    container_port: 80   # Port inside the container
    protocol: "tcp"      # Protocol: tcp or udp
```

**Important**: If `host_port` is not specified or set to 0, Docker will assign a random port.

#### Resources (resources)

| Field | Type | Required | Minimum | Example | Description |
|-------|------|----------|---------|---------|-------------|
| `cpu_millicores` | `integer` | **Yes** | `5` | `500` | CPU in millicores (1000m = 1 core) |
| `memory_bytes` | `integer` | **Yes** | `16MB` | `268435456` | Memory in bytes |

**Notes:**
- 1 CPU core = 1000 millicores
- 100m = 0.1 CPU core
- Minimum memory: 16MB (16,777,216 bytes)

#### Auto-scaling Policy (scale_policy)

| Field | Type | Required | Minimum | Example | Description |
|-------|------|----------|---------|---------|-------------|
| `min_replicas` | `integer` | No | `1` | `1` | Minimum number of replicas |
| `max_replicas` | `integer` | No | `1` | `5` | Maximum number of replicas |
| `target_cpu` | `float` | No | `5.0` | `70.0` | Target CPU utilization percentage |
| `target_memory` | `float` | No | `10.0` | `80.0` | Target memory utilization percentage |
| `cooldown_seconds` | `integer` | No | `30` | `60` | Cooldown period between scaling actions (seconds) |

#### Health Check (health_check)

| Field | Type | Required | Depends on type | Example | Description |
|-------|------|----------|----------------|---------|-------------|
| `type` | `string` | No | - | `"http"` | Check type: `http`, `tcp`, `command` |
| `http_path` | `string` | For `type: http` | - | `"/health"` | URL path for HTTP check |
| `port` | `integer` | For `type: http/tcp` | - | `8080` | Port for checking |
| `command` | `array` | For `type: command` | - | `["curl", "-f", "http://localhost:3000/health"]` | Command to execute |
| `interval` | `integer` | No | `1` | `30` | Interval between checks (seconds) |
| `timeout` | `integer` | No | `0` | `10` | Check timeout (seconds) |
| `retries` | `integer` | No | `0` | `3` | Number of retries before marking as unhealthy |

## Configuration Examples

### Example 1: Web Server

```yaml
service_name: "web-frontend"
image: "nginx:alpine"
replicas: 3
ports:
  - host_port: 80
    container_port: 80
    protocol: "tcp"
env:
  - "NGINX_ENV=production"
resources:
  cpu_millicores: 200    # 0.2 CPU cores
  memory_bytes: 134217728  # 128MB
health_check:
  type: "http"
  http_path: "/health"
  port: 80
  interval: 30
```

### Example 2: Database

```yaml
service_name: "postgres-db"
image: "postgres:15-alpine"
replicas: 1
ports:
  - host_port: 5432
    container_port: 5432
env:
  - "POSTGRES_PASSWORD=secret"
volumes:
  - "pgdata:/var/lib/postgresql/data"
resources:
  cpu_millicores: 500    # 0.5 CPU cores
  memory_bytes: 1073741824  # 1GB
health_check:
  type: "command"
  command: ["pg_isready", "-U", "postgres"]
  interval: 30
```

### Example 3: Application with Auto-scaling

```yaml
service_name: "api-service"
image: "myapp/api:latest"
replicas: 2
ports:
  - host_port: 3000
    container_port: 3000
scale_policy:
  min_replicas: 2
  max_replicas: 10
  target_cpu: 80.0
  cooldown_seconds: 120
health_check:
  type: "tcp"
  port: 3000
  interval: 15
```

## Additional Notes

### Environment Variables (env)

```yaml
env:
  - "DATABASE_URL=postgresql://user:pass@db:5432/app"
  - "LOG_LEVEL=debug"
  - "REDIS_HOST=redis"
```

### Volumes

Three formats are supported:
1. **Bind mount**: `/host/path:/container/path`
2. **Named volume**: `volume_name:/container/path`
3. **Read-only**: `/host/path:/container/path:ro`

```yaml
volumes:
  - "/var/www:/usr/share/nginx/html"  # Bind mount
  - "app_data:/data"                  # Named volume
  - "/config:/etc/app:ro"             # Read-only
```

### Restart Policies (restart_policy)

| Value | Description |
|-------|-------------|
| `no` | Do not restart (default) |
| `always` | Always restart |
| `on-failure` | Restart on failure |
| `unless-stopped` | Restart unless manually stopped |

### Units of Measurement

| Resource | Unit | Example | Note |
|----------|------|---------|------|
| CPU | millicores | `500` = 0.5 core | 1000m = 1 full core |
| Memory | bytes | `268435456` = 256MB | Use powers of two |
| Time | seconds | `30` = 30 seconds | For intervals and timeouts |

## Complete Example

```yaml
# config.yaml
env: "production"
listen_addr: ":8080"
data_dir: "/var/lib/gorchester"
cluster_name: "production-cluster"

services:
  - service_name: "load-balancer"
    image: "nginx:alpine"
    replicas: 2
    ports:
      - host_port: 80
        container_port: 80
        protocol: "tcp"
      - host_port: 443
        container_port: 443
        protocol: "tcp"
    env:
      - "UPSTREAM_SERVERS=app1:3000,app2:3000"
    volumes:
      - "/etc/nginx/conf.d:/etc/nginx/conf.d:ro"
    resources:
      cpu_millicores: 300
      memory_bytes: 134217728
    scale_policy:
      min_replicas: 2
      max_replicas: 5
      target_cpu: 70.0
      cooldown_seconds: 90
    health_check:
      type: "http"
      http_path: "/nginx-health"
      port: 80
      interval: 30
      timeout: 5
      retries: 3

  - service_name: "application"
    image: "mycompany/app:1.5.0"
    replicas: 3
    ports:
      - host_port: 0  # Random port
        container_port: 3000
    env:
      - "DATABASE_URL=postgresql://user:pass@db:5432/app"
      - "REDIS_URL=redis://cache:6379/0"
    resources:
      cpu_millicores: 500
      memory_bytes: 268435456
    scale_policy:
      min_replicas: 3
      max_replicas: 10
      target_cpu: 75.0
      target_memory: 85.0
      cooldown_seconds: 120
    health_check:
      type: "http"
      http_path: "/api/health"
      port: 3000
      interval: 20
      timeout: 10
      retries: 2
```

## Validation

The orchestrator automatically validates the configuration:
- CPU must be at least 5 millicores
- Memory must be at least 16MB
- Valid service names
- Required fields are specified

To validate configuration without starting the orchestrator:
```bash
go run cmd/validator/main.go --config config.yaml
```

*Configuration Version: 1.0*

### Running the Master

*(Instruction will be added once the basic API is stable)*
```bash
./go-orchestrator agent --master-url=<master-address>
```

## 🤝 Contributing
As this is a Master's thesis project, the primary development is done by me. However, constructive feedback, suggestions, and discussions are highly welcome! Please feel free to open an issue to start a conversation.

## 📜 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 📚 Academic Context
This project is developed as a core component of my Master's thesis in "Computer Science" at SBMPEI. The focus is on researching and implementing scheduling algorithms and distributed systems principles.



<div align="center">
Happy Coding! 👨‍💻

</div>