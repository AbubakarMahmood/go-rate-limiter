.PHONY: help build run test benchmark lint fmt clean docker-up docker-down load-test

# Variables
BINARY_NAME=rate-limiter
DOCKER_COMPOSE=docker-compose -f docker/docker-compose.yml

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the application
	@echo "Building $(BINARY_NAME)..."
	@go build -o bin/$(BINARY_NAME) cmd/server/main.go
	@echo "Build complete: bin/$(BINARY_NAME)"

run: ## Run the application
	@echo "Running $(BINARY_NAME)..."
	@go run cmd/server/main.go

test: ## Run unit tests
	@echo "Running tests..."
	@go test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	@./scripts/benchmark.sh

benchmark-detail: ## Run detailed benchmarks
	@echo "Running detailed benchmarks..."
	@go test -bench=. -benchmem -benchtime=30s -cpuprofile=cpu.prof -memprofile=mem.prof ./tests/benchmark/
	@echo "CPU profile: cpu.prof"
	@echo "Memory profile: mem.prof"

lint: ## Run linter
	@echo "Running linter..."
	@golangci-lint run ./... || echo "Install golangci-lint: https://golangci-lint.run/usage/install/"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w . || echo "Install goimports: go install golang.org/x/tools/cmd/goimports@latest"

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	@go mod tidy

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@rm -f cpu.prof mem.prof
	@rm -f benchmark-results.txt
	@rm -rf load-test-results/
	@echo "Clean complete"

install-deps: ## Install Go dependencies
	@echo "Installing dependencies..."
	@go mod download

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -f docker/Dockerfile -t rate-limiter:latest .

docker-up: ## Start Docker Compose stack
	@echo "Starting Docker Compose stack..."
	@$(DOCKER_COMPOSE) up -d
	@echo "Stack started. Services available at:"
	@echo "  - Rate Limiter: http://localhost:8080"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Grafana: http://localhost:3000 (admin/admin)"

docker-down: ## Stop Docker Compose stack
	@echo "Stopping Docker Compose stack..."
	@$(DOCKER_COMPOSE) down

docker-logs: ## View Docker Compose logs
	@$(DOCKER_COMPOSE) logs -f

docker-restart: docker-down docker-up ## Restart Docker Compose stack

load-test: ## Run load test
	@echo "Running load test..."
	@./scripts/load-test.sh

load-test-heavy: ## Run heavy load test
	@echo "Running heavy load test..."
	@./scripts/load-test.sh 60s 10000

stress-test: ## Run stress test
	@echo "Running stress test..."
	@./scripts/load-test.sh 120s 50000

all: fmt vet lint test build ## Run all checks and build

ci: fmt vet test ## Run CI checks

install-tools: ## Install development tools
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/tsenart/vegeta@latest
	@echo "Tools installed"

.DEFAULT_GOAL := help
