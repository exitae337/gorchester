.PHONY: run clean test build

# Run with cleaning old containers (managed by gorchester)
run: clean
	go run cmd/gorchester/main.go

# Clean old containers
clean:
	@echo "🧹 Cleaning up old containers..."
	@docker ps -a --filter "label=managed-by=gorchester" -q | xargs -r docker stop || true
	@docker ps -a --filter "label=managed-by=gorchester" -q | xargs -r docker rm || true
	@docker ps -a --filter "name=gorchester-" -q | xargs -r docker stop || true
	@docker ps -a --filter "name=gorchester-" -q | xargs -r docker rm || true
	@echo "✅ Cleanup done"

# Full cleaning (with images)
clean-all: clean
	@echo "🧹 Removing gorchester images..."
	@docker images --filter "reference=gorchester-*" -q | xargs -r docker rmi || true

# Run tests
test:
	go test -v ./...

# Build
build:
	go build -o bin/gorchester cmd/gorchester/main.go

# Force start
run-fast:
	go run cmd/gorchester/main.go

# Help
help:
	@echo "Available commands:"
	@echo "  make run        - Run with cleanup"
	@echo "  make clean      - Remove old containers"
	@echo "  make clean-all  - Remove containers and images"
	@echo "  make run-fast   - Run without cleanup"
	@echo "  make build      - Build binary"
	@echo "  make test       - Run tests"