.PHONY: help build run test test-coverage bench fmt vet tidy clean docker-build docker-up docker-down docker-logs load-test

BINARY_NAME=rate-limiter
DOCKER_COMPOSE=docker compose -f docker/docker-compose.yml

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the server binary
	go build -trimpath -o bin/$(BINARY_NAME) ./cmd/server

run: ## Run the server
	go run ./cmd/server

test: ## Run tests with the race detector
	go test -race -count=1 ./...

test-coverage: ## Run tests and write an HTML coverage report
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report: coverage.html"

bench: ## Run benchmarks
	go test -bench=. -benchmem -run='^$$' ./internal/algorithms/

fmt: ## Format code
	gofmt -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go modules
	go mod tidy

clean: ## Remove build and test artifacts
	rm -rf bin/ load-test-results/
	rm -f coverage.out coverage.html cpu.prof mem.prof

docker-build: ## Build the Docker image
	docker build -f docker/Dockerfile -t rate-limiter:latest .

docker-up: ## Start the full stack (service, Redis, Prometheus, Grafana)
	$(DOCKER_COMPOSE) up -d --build
	@echo "rate limiter: http://localhost:8081"
	@echo "prometheus:   http://localhost:9090"
	@echo "grafana:      http://localhost:3000 (admin/admin)"

docker-down: ## Stop the stack
	$(DOCKER_COMPOSE) down

docker-logs: ## Tail stack logs
	$(DOCKER_COMPOSE) logs -f

load-test: ## Run a vegeta load test against a running instance
	./scripts/load-test.sh

.DEFAULT_GOAL := help
