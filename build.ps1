# Go Rate Limiter - PowerShell Build Script
# Windows alternative to Makefile

param(
    [Parameter(Position=0)]
    [string]$Command = "help"
)

$BinaryName = "rate-limiter.exe"
$DockerCompose = "docker-compose -f docker/docker-compose.yml"

function Show-Help {
    Write-Host "Go Rate Limiter - Build Commands" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Usage: .\build.ps1 [command]" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Available commands:" -ForegroundColor Green
    Write-Host "  help           - Show this help message"
    Write-Host "  build          - Build the application"
    Write-Host "  run            - Run the application"
    Write-Host "  test           - Run unit tests"
    Write-Host "  test-coverage  - Run tests with coverage"
    Write-Host "  benchmark      - Run benchmarks"
    Write-Host "  fmt            - Format code"
    Write-Host "  vet            - Run go vet"
    Write-Host "  clean          - Clean build artifacts"
    Write-Host "  docker-build   - Build Docker image"
    Write-Host "  docker-up      - Start Docker Compose stack"
    Write-Host "  docker-down    - Stop Docker Compose stack"
    Write-Host "  docker-logs    - View Docker logs"
    Write-Host "  load-test      - Run load test"
    Write-Host ""
}

function Build-App {
    Write-Host "Building $BinaryName..." -ForegroundColor Green
    go build -o "bin/$BinaryName" cmd/server/main.go
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Build complete: bin/$BinaryName" -ForegroundColor Green
    } else {
        Write-Host "Build failed!" -ForegroundColor Red
        exit 1
    }
}

function Run-App {
    Write-Host "Running $BinaryName..." -ForegroundColor Green
    go run cmd/server/main.go
}

function Run-Tests {
    Write-Host "Running tests..." -ForegroundColor Green
    go test -v -race -cover ./...
}

function Run-TestCoverage {
    Write-Host "Running tests with coverage..." -ForegroundColor Green
    go test -v -race -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    Write-Host "Coverage report: coverage.html" -ForegroundColor Green
    Start-Process coverage.html
}

function Run-Benchmark {
    Write-Host "Running benchmarks..." -ForegroundColor Green
    go test -bench=. -benchmem -benchtime=10s ./tests/benchmark/
}

function Format-Code {
    Write-Host "Formatting code..." -ForegroundColor Green
    go fmt ./...
}

function Run-Vet {
    Write-Host "Running go vet..." -ForegroundColor Green
    go vet ./...
}

function Clean-Artifacts {
    Write-Host "Cleaning..." -ForegroundColor Green
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue bin/
    Remove-Item -Force -ErrorAction SilentlyContinue coverage.out, coverage.html
    Remove-Item -Force -ErrorAction SilentlyContinue cpu.prof, mem.prof
    Remove-Item -Force -ErrorAction SilentlyContinue benchmark-results.txt
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue load-test-results/
    Write-Host "Clean complete" -ForegroundColor Green
}

function Docker-Build {
    Write-Host "Building Docker image..." -ForegroundColor Green
    docker build -f docker/Dockerfile -t rate-limiter:latest .
}

function Docker-Up {
    Write-Host "Starting Docker Compose stack..." -ForegroundColor Green
    Invoke-Expression "$DockerCompose up -d"
    Write-Host ""
    Write-Host "Services available at:" -ForegroundColor Cyan
    Write-Host "  - Rate Limiter: http://localhost:8081" -ForegroundColor Yellow
    Write-Host "  - Prometheus:   http://localhost:9090" -ForegroundColor Yellow
    Write-Host "  - Grafana:      http://localhost:3000 (admin/admin)" -ForegroundColor Yellow
}

function Docker-Down {
    Write-Host "Stopping Docker Compose stack..." -ForegroundColor Green
    Invoke-Expression "$DockerCompose down"
}

function Docker-Logs {
    Write-Host "Viewing Docker logs..." -ForegroundColor Green
    Invoke-Expression "$DockerCompose logs -f"
}

function Run-LoadTest {
    Write-Host "Running load test..." -ForegroundColor Green
    Write-Host "Note: Load testing requires bash (Git Bash or WSL)" -ForegroundColor Yellow
    bash scripts/load-test.sh
}

# Main command dispatcher
switch ($Command.ToLower()) {
    "help"           { Show-Help }
    "build"          { Build-App }
    "run"            { Run-App }
    "test"           { Run-Tests }
    "test-coverage"  { Run-TestCoverage }
    "benchmark"      { Run-Benchmark }
    "fmt"            { Format-Code }
    "vet"            { Run-Vet }
    "clean"          { Clean-Artifacts }
    "docker-build"   { Docker-Build }
    "docker-up"      { Docker-Up }
    "docker-down"    { Docker-Down }
    "docker-logs"    { Docker-Logs }
    "load-test"      { Run-LoadTest }
    default {
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Write-Host "Run '.\build.ps1 help' for usage" -ForegroundColor Yellow
        exit 1
    }
}
