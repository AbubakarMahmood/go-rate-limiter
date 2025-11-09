# 🚀 Quick Start Guide

Everything you need to use the Go Rate Limiter in 5 minutes.

## Table of Contents
- [Starting & Stopping](#starting--stopping)
- [Testing the API](#testing-the-api)
- [Common Commands](#common-commands)
- [PowerShell Examples](#powershell-examples)
- [Prometheus Queries](#prometheus-queries)
- [Troubleshooting](#troubleshooting)

---

## Starting & Stopping

### Start Everything
```powershell
.\build.ps1 docker-up
```

**What starts:**
- Rate Limiter API: http://localhost:8081
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)
- Redis: localhost:6379

### Stop Everything
```powershell
.\build.ps1 docker-down
```

### View Logs
```powershell
.\build.ps1 docker-logs
```

---

## Testing the API

### Quick Test (All Endpoints)
```powershell
.\test-api.ps1
```

### Manual Health Check
```powershell
Invoke-WebRequest http://localhost:8081/health | Select -Expand Content
```

---

## Common Commands

### PowerShell Build Script

| Command | What It Does |
|---------|--------------|
| `.\build.ps1 help` | Show all commands |
| `.\build.ps1 docker-up` | Start all services |
| `.\build.ps1 docker-down` | Stop all services |
| `.\build.ps1 docker-logs` | View logs |
| `.\build.ps1 docker-restart` | Restart everything |
| `.\test-api.ps1` | Run API tests |

---

## PowerShell Examples

### 1. Check Rate Limit (Default Algorithm)
```powershell
$body = @{
    resource = "api.users.create"
    identifier = "user-123"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

### 2. Use Token Bucket Algorithm
```powershell
$body = @{
    resource = "api.posts"
    identifier = "user-456"
    algorithm = "token_bucket"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

### 3. Use Sliding Window Algorithm
```powershell
$body = @{
    resource = "api.comments"
    identifier = "user-789"
    algorithm = "sliding_window"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

### 4. Use Fixed Window Algorithm
```powershell
$body = @{
    resource = "api.messages"
    identifier = "user-999"
    algorithm = "fixed_window"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

### 5. Consume Multiple Tokens at Once
```powershell
$body = @{
    resource = "api.bulk.upload"
    identifier = "user-123"
    count = 10  # Consume 10 tokens
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

### 6. Check Status (Without Consuming Tokens)
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/status/user-123:api.users.create?algorithm=token_bucket"
```

### 7. Reset Rate Limit (Admin)
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/reset/user-123:api.users.create?algorithm=token_bucket" `
    -Method Post
```

### 8. Test Rate Limiting (Send 150 Requests)
```powershell
# This will hit the limit and get denied
1..150 | ForEach-Object {
    $body = @{
        resource = "api.test"
        identifier = "spam-user"
    } | ConvertTo-Json

    try {
        $result = Invoke-RestMethod -Uri http://localhost:8081/v1/check `
            -Method Post -Body $body -ContentType "application/json" -ErrorAction Stop
        Write-Host "Request $_ : ✓ Allowed (Remaining: $($result.remaining))" -ForegroundColor Green
    } catch {
        Write-Host "Request $_ : ✗ Rate Limited!" -ForegroundColor Red
    }
}
```

---

## Prometheus Queries

Open http://localhost:9090 and try these queries:

### Total Requests
```promql
rate_limiter_requests_total
```

### Requests Allowed
```promql
rate_limiter_requests_allowed
```

### Requests Denied
```promql
rate_limiter_requests_denied
```

### Requests per Second
```promql
rate(rate_limiter_requests_total[1m])
```

### Average Latency
```promql
rate(rate_limiter_latency_seconds_sum[1m]) / rate(rate_limiter_latency_seconds_count[1m])
```

### P95 Latency
```promql
histogram_quantile(0.95, rate(rate_limiter_latency_seconds_bucket[1m]))
```

### Denied Requests Rate
```promql
rate(rate_limiter_requests_denied[1m])
```

---

## API Endpoints Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/check` | Check if request is allowed |
| GET | `/v1/status/:key` | Get current limit status |
| POST | `/v1/reset/:key` | Reset limits for a key |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |

---

## Algorithm Comparison

| Algorithm | Limit | Burst | Memory | Accuracy | Best For |
|-----------|-------|-------|--------|----------|----------|
| **Token Bucket** | 120/min | Yes | Low | Good | Smooth traffic, allows bursts |
| **Sliding Window** | 100/min | No | Medium | High | Strict rate limiting |
| **Fixed Window** | 100/min | No | Lowest | Low | Simple, fast operations |

---

## Configuration

Edit `config.yaml` to change limits:

```yaml
limits:
  default:
    requests: 100   # Change this
    window: 1m      # Change this
    burst: 120      # For token bucket

  tiers:
    free:
      requests: 100
      window: 1h
    premium:
      requests: 10000
      window: 1h
```

After changing config, restart:
```powershell
.\build.ps1 docker-restart
```

---

## Troubleshooting

### Port 8081 Already in Use
```powershell
# Find what's using it
netstat -ano | findstr :8081

# Kill the process
taskkill /PID <PID> /F
```

### Services Won't Start
```powershell
# Clean everything and restart
.\build.ps1 docker-down
docker system prune -f
.\build.ps1 docker-up
```

### Can't Connect to API
```powershell
# Check if containers are running
docker ps

# Check logs
.\build.ps1 docker-logs
```

### Redis Connection Error
```powershell
# Restart just Redis
docker restart rate-limiter-redis

# Check Redis logs
docker logs rate-limiter-redis
```

---

## Next Steps

1. **View Metrics**: Open http://localhost:9090
2. **Create Dashboards**: Open http://localhost:3000 (admin/admin)
3. **Customize Limits**: Edit `config.yaml`
4. **Read Full Docs**: See `README.md`

---

## Quick Reference Card

Save this for easy access:

```
START:   .\build.ps1 docker-up
STOP:    .\build.ps1 docker-down
TEST:    .\test-api.ps1
LOGS:    .\build.ps1 docker-logs

API:     http://localhost:8081
PROM:    http://localhost:9090
GRAF:    http://localhost:3000

HEALTH:  http://localhost:8081/health
METRICS: http://localhost:8081/metrics
```
