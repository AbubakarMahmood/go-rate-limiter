# 📋 Rate Limiter Cheat Sheet

Quick reference for the most common operations.

---

## 🚀 Start/Stop

```powershell
.\build.ps1 docker-up      # Start everything
.\build.ps1 docker-down    # Stop everything
.\build.ps1 docker-logs    # View logs
.\test-api.ps1             # Run tests
```

---

## 🌐 URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| API | http://localhost:8081 | - |
| Prometheus | http://localhost:9090 | - |
| Grafana | http://localhost:3000 | admin/admin |
| Health | http://localhost:8081/health | - |
| Metrics | http://localhost:8081/metrics | - |

---

## 📡 API Calls

### Basic Request
```powershell
$body = @{ resource = "api.test"; identifier = "user-123" } | ConvertTo-Json
Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json"
```

### With Algorithm
```powershell
$body = @{ resource = "api.test"; identifier = "user-123"; algorithm = "token_bucket" } | ConvertTo-Json
Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json"
```

### Multiple Tokens
```powershell
$body = @{ resource = "api.test"; identifier = "user-123"; count = 10 } | ConvertTo-Json
Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json"
```

### Check Status
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/status/user-123:api.test"
```

### Reset Limit
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/reset/user-123:api.test" -Method Post
```

### Health Check
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/health"
```

---

## 🎛️ Algorithms

| Name | Limit | Best For | Config Key |
|------|-------|----------|------------|
| Token Bucket | 120/min | Smooth traffic + bursts | `token_bucket` |
| Sliding Window | 100/min | Strict limiting | `sliding_window` |
| Fixed Window | 100/min | Simple & fast | `fixed_window` |

---

## 📊 Prometheus Queries

Copy these into http://localhost:9090

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

### Requests/Second
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

---

## ⚙️ Configuration

Edit `config.yaml`:

```yaml
limits:
  default:
    requests: 100    # ← Change this
    window: 1m       # ← Change this
    burst: 120       # ← Token bucket only

  tiers:
    free:
      requests: 100
      window: 1h
```

Restart after changing:
```powershell
.\build.ps1 docker-restart
```

---

## 🐛 Troubleshooting

### Port Already in Use
```powershell
netstat -ano | findstr :8081
taskkill /PID <PID> /F
```

### Clean Reset
```powershell
.\build.ps1 docker-down
docker system prune -f
.\build.ps1 docker-up
```

### View Logs
```powershell
docker logs rate-limiter-service
docker logs rate-limiter-redis
```

### Check If Running
```powershell
docker ps
```

---

## 🧪 Testing

### Simple Test
```powershell
.\test-api.ps1
```

### Spam Test (Hit Limit)
```powershell
1..150 | % {
    $body = @{ resource = "api.test"; identifier = "spam" } | ConvertTo-Json
    Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json" -ErrorAction SilentlyContinue
}
```

### Health Check
```powershell
Invoke-WebRequest http://localhost:8081/health | Select -Expand Content
```

---

## 📁 Files

| File | Purpose |
|------|---------|
| `QUICKSTART.md` | Detailed quick start guide |
| `EXAMPLES.md` | Real-world code examples |
| `CHEATSHEET.md` | This file - quick reference |
| `README.md` | Full documentation |
| `build.ps1` | PowerShell build script |
| `test-api.ps1` | API test suite |
| `config.yaml` | Configuration file |

---

## 🎯 Common Workflows

### Test Rate Limiting
```powershell
# 1. Start services
.\build.ps1 docker-up

# 2. Run tests
.\test-api.ps1

# 3. View metrics
Start-Process http://localhost:9090
```

### Change Limits
```powershell
# 1. Edit config.yaml
notepad config.yaml

# 2. Restart
.\build.ps1 docker-restart

# 3. Test new limits
.\test-api.ps1
```

### Monitor Performance
```powershell
# 1. Open Prometheus
Start-Process http://localhost:9090

# 2. Run this query:
#    rate(rate_limiter_requests_total[1m])

# 3. View in Grafana
Start-Process http://localhost:3000
```

---

## 💡 Tips

- **Default algorithm**: token_bucket
- **Default limit**: 120 requests/minute (burst)
- **Health check**: Always returns 200 if running
- **Metrics**: Updated in real-time
- **Redis**: Auto-cleanup every 24 hours
- **Logs**: Use `docker logs rate-limiter-service`

---

## 🆘 Need Help?

1. Check `QUICKSTART.md` for detailed instructions
2. See `EXAMPLES.md` for code samples
3. Read `README.md` for full documentation
4. Run `.\build.ps1 help` for commands
5. Run `.\test-api.ps1` to verify it's working

---

**Quick Test:**
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/health"
# Should return: {"status":"healthy","time":"..."}
```

**If that works, you're good to go!** ✅
