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