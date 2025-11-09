# 📚 API Examples

Real-world examples for common use cases.

## Table of Contents
- [Basic Usage](#basic-usage)
- [Advanced Scenarios](#advanced-scenarios)
- [Integration Examples](#integration-examples)
- [Production Patterns](#production-patterns)

---

## Basic Usage

### Example 1: Simple API Rate Limiting

```powershell
# Protect your API endpoint
$body = @{
    resource = "api.users.create"
    identifier = "user-email@example.com"
} | ConvertTo-Json

$response = Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"

if ($response.allowed) {
    Write-Host "✓ Request allowed. Remaining: $($response.remaining)"
    # Process the request...
} else {
    Write-Host "✗ Rate limited. Retry after: $($response.retry_after) seconds"
}
```

### Example 2: Different Limits for Different Users

```powershell
# Free tier user (100 requests/hour)
$body = @{
    resource = "api.search"
    identifier = "free-user-123"
    algorithm = "fixed_window"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"

# Premium user (10,000 requests/hour)
$body = @{
    resource = "api.search"
    identifier = "premium-user-456"
    algorithm = "token_bucket"
} | ConvertTo-Json

Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"
```

---

## Advanced Scenarios

### Example 3: Bulk Operations

```powershell
# User wants to upload 50 files - consume 50 tokens at once
$body = @{
    resource = "api.files.upload"
    identifier = "user-789"
    count = 50  # Reserve 50 tokens
    algorithm = "token_bucket"
} | ConvertTo-Json

$response = Invoke-RestMethod -Uri http://localhost:8081/v1/check `
    -Method Post -Body $body -ContentType "application/json"

if ($response.allowed) {
    Write-Host "✓ Bulk upload approved for 50 files"
    # Process bulk upload...
} else {
    Write-Host "✗ Insufficient tokens. Only $($response.remaining) available"
}
```

### Example 4: Check Before Acting

```powershell
# Check current status without consuming tokens
$status = Invoke-RestMethod -Uri "http://localhost:8081/v1/status/user-123:api.expensive.operation"

Write-Host "Current status:"
Write-Host "  Limit: $($status.limit)"
Write-Host "  Remaining: $($status.remaining)"
Write-Host "  Resets at: $($status.reset_at)"

if ($status.remaining -gt 10) {
    # Safe to proceed with expensive operation
    Write-Host "✓ Sufficient quota remaining"
} else {
    Write-Host "⚠ Low quota, consider waiting"
}
```

### Example 5: Gradual Backoff

```powershell
function Invoke-RateLimitedRequest {
    param($Identifier, $Resource)

    $maxRetries = 3
    $retryDelay = 1  # seconds

    for ($i = 0; $i -lt $maxRetries; $i++) {
        $body = @{
            resource = $Resource
            identifier = $Identifier
        } | ConvertTo-Json

        try {
            $response = Invoke-RestMethod -Uri http://localhost:8081/v1/check `
                -Method Post -Body $body -ContentType "application/json"

            if ($response.allowed) {
                Write-Host "✓ Request succeeded"
                return $response
            } else {
                $waitTime = if ($response.retry_after) { $response.retry_after } else { $retryDelay }
                Write-Host "⏳ Rate limited. Waiting $waitTime seconds..."
                Start-Sleep -Seconds $waitTime
                $retryDelay *= 2  # Exponential backoff
            }
        } catch {
            Write-Host "❌ Error: $_"
            Start-Sleep -Seconds $retryDelay
            $retryDelay *= 2
        }
    }

    Write-Host "❌ Max retries exceeded"
    return $null
}

# Usage
Invoke-RateLimitedRequest -Identifier "user-123" -Resource "api.data.fetch"
```

---

## Integration Examples

### Example 6: C# Integration

```csharp
using System.Net.Http;
using System.Text;
using System.Text.Json;

public class RateLimiterClient
{
    private readonly HttpClient _client;
    private readonly string _baseUrl = "http://localhost:8081";

    public async Task<RateLimitResponse> CheckAsync(string resource, string identifier)
    {
        var request = new
        {
            resource = resource,
            identifier = identifier
        };

        var content = new StringContent(
            JsonSerializer.Serialize(request),
            Encoding.UTF8,
            "application/json"
        );

        var response = await _client.PostAsync($"{_baseUrl}/v1/check", content);
        var json = await response.Content.ReadAsStringAsync();

        return JsonSerializer.Deserialize<RateLimitResponse>(json);
    }
}

public class RateLimitResponse
{
    public bool Allowed { get; set; }
    public int Limit { get; set; }
    public int Remaining { get; set; }
    public DateTime ResetAt { get; set; }
    public int? RetryAfter { get; set; }
}

// Usage
var limiter = new RateLimiterClient();
var result = await limiter.CheckAsync("api.users.create", "user@example.com");

if (result.Allowed)
{
    // Process request
}
else
{
    // Handle rate limit
    Thread.Sleep(result.RetryAfter ?? 1000);
}
```

### Example 7: Python Integration

```python
import requests
import time

class RateLimiter:
    def __init__(self, base_url="http://localhost:8081"):
        self.base_url = base_url

    def check(self, resource, identifier, algorithm="token_bucket", count=1):
        """Check if request is allowed"""
        response = requests.post(
            f"{self.base_url}/v1/check",
            json={
                "resource": resource,
                "identifier": identifier,
                "algorithm": algorithm,
                "count": count
            }
        )
        return response.json()

    def check_with_retry(self, resource, identifier, max_retries=3):
        """Check with automatic retry on rate limit"""
        for attempt in range(max_retries):
            result = self.check(resource, identifier)

            if result["allowed"]:
                return True

            retry_after = result.get("retry_after", 1)
            print(f"Rate limited. Waiting {retry_after}s...")
            time.sleep(retry_after)

        return False

# Usage
limiter = RateLimiter()

if limiter.check_with_retry("api.data", "user-123"):
    # Process request
    print("Request allowed!")
else:
    print("Rate limit exceeded")
```

### Example 8: Node.js Integration

```javascript
const axios = require('axios');

class RateLimiter {
    constructor(baseUrl = 'http://localhost:8081') {
        this.baseUrl = baseUrl;
    }

    async check(resource, identifier, options = {}) {
        const response = await axios.post(`${this.baseUrl}/v1/check`, {
            resource,
            identifier,
            algorithm: options.algorithm || 'token_bucket',
            count: options.count || 1
        });

        return response.data;
    }

    async checkWithRetry(resource, identifier, maxRetries = 3) {
        for (let i = 0; i < maxRetries; i++) {
            try {
                const result = await this.check(resource, identifier);

                if (result.allowed) {
                    return { success: true, data: result };
                }

                const retryAfter = result.retry_after || 1;
                console.log(`Rate limited. Waiting ${retryAfter}s...`);
                await new Promise(resolve => setTimeout(resolve, retryAfter * 1000));

            } catch (error) {
                console.error(`Attempt ${i + 1} failed:`, error.message);
            }
        }

        return { success: false };
    }
}

// Usage
const limiter = new RateLimiter();

const result = await limiter.checkWithRetry('api.users', 'user-123');
if (result.success) {
    // Process request
    console.log('Request allowed!');
} else {
    console.log('Rate limit exceeded');
}
```

---

## Production Patterns

### Example 9: Multi-Tier Rate Limiting

```powershell
function Get-UserTier {
    param($UserId)
    # In production, fetch from database
    return "premium"  # or "free", "enterprise"
}

function Check-RateLimit {
    param($UserId, $Resource)

    $tier = Get-UserTier -UserId $UserId

    # Different algorithms for different tiers
    $algorithm = switch ($tier) {
        "free" { "fixed_window" }
        "premium" { "token_bucket" }
        "enterprise" { "sliding_window" }
        default { "token_bucket" }
    }

    $body = @{
        resource = "$tier.$Resource"  # Separate limits per tier
        identifier = $UserId
        algorithm = $algorithm
    } | ConvertTo-Json

    Invoke-RestMethod -Uri http://localhost:8081/v1/check `
        -Method Post -Body $body -ContentType "application/json"
}

# Usage
$result = Check-RateLimit -UserId "user-123" -Resource "api.search"
```

### Example 10: Monitoring and Alerting

```powershell
# Check rate limit health
function Test-RateLimiterHealth {
    try {
        $health = Invoke-RestMethod -Uri http://localhost:8081/health

        if ($health.status -eq "healthy") {
            Write-Host "✓ Rate limiter is healthy" -ForegroundColor Green
            return $true
        }
    } catch {
        Write-Host "✗ Rate limiter is down!" -ForegroundColor Red
        # Send alert...
        return $false
    }
}

# Monitor denied requests
function Get-DeniedRequestsRate {
    $metrics = Invoke-WebRequest -Uri http://localhost:8081/metrics |
        Select -Expand Content

    # Parse Prometheus metrics
    $deniedLine = $metrics -split "`n" |
        Where-Object { $_ -match "rate_limiter_requests_denied" -and $_ -notmatch "#" }

    if ($deniedLine) {
        Write-Host "Denied requests: $deniedLine"
    }
}

# Usage
Test-RateLimiterHealth
Get-DeniedRequestsRate
```

### Example 11: Circuit Breaker Pattern

```powershell
class CircuitBreaker {
    [int]$FailureThreshold = 5
    [int]$Timeout = 60
    [int]$FailureCount = 0
    [datetime]$LastFailureTime
    [string]$State = "Closed"  # Closed, Open, HalfOpen

    [bool] IsAvailable() {
        if ($this.State -eq "Open") {
            if ((Get-Date) -gt $this.LastFailureTime.AddSeconds($this.Timeout)) {
                $this.State = "HalfOpen"
                return $true
            }
            return $false
        }
        return $true
    }

    [void] RecordSuccess() {
        $this.FailureCount = 0
        $this.State = "Closed"
    }

    [void] RecordFailure() {
        $this.FailureCount++
        $this.LastFailureTime = Get-Date

        if ($this.FailureCount -ge $this.FailureThreshold) {
            $this.State = "Open"
            Write-Host "⚠ Circuit breaker opened!" -ForegroundColor Yellow
        }
    }
}

$breaker = [CircuitBreaker]::new()

function Invoke-ProtectedRequest {
    param($Resource, $Identifier)

    if (-not $breaker.IsAvailable()) {
        Write-Host "✗ Circuit breaker is open" -ForegroundColor Red
        return $null
    }

    try {
        $body = @{ resource = $Resource; identifier = $Identifier } | ConvertTo-Json
        $response = Invoke-RestMethod -Uri http://localhost:8081/v1/check `
            -Method Post -Body $body -ContentType "application/json"

        $breaker.RecordSuccess()
        return $response
    } catch {
        $breaker.RecordFailure()
        Write-Host "✗ Request failed: $_" -ForegroundColor Red
        return $null
    }
}
```

---

## Testing Examples

### Example 12: Load Testing

```powershell
# Simple load test
function Invoke-LoadTest {
    param(
        [int]$Requests = 1000,
        [int]$Concurrent = 10
    )

    $results = @{
        Allowed = 0
        Denied = 0
        Errors = 0
    }

    Write-Host "Starting load test: $Requests requests, $Concurrent concurrent"

    $jobs = @()
    for ($i = 0; $i -lt $Requests; $i++) {
        $jobs += Start-Job -ScriptBlock {
            param($BaseUrl, $RequestId)

            $body = @{
                resource = "api.load.test"
                identifier = "load-test-$RequestId"
            } | ConvertTo-Json

            try {
                $response = Invoke-RestMethod -Uri "$BaseUrl/v1/check" `
                    -Method Post -Body $body -ContentType "application/json"

                return @{ Status = if ($response.allowed) { "Allowed" } else { "Denied" } }
            } catch {
                return @{ Status = "Error" }
            }
        } -ArgumentList "http://localhost:8081", ($i % 100)

        # Limit concurrent jobs
        while ((Get-Job -State Running).Count -ge $Concurrent) {
            Start-Sleep -Milliseconds 10
        }
    }

    # Wait for all jobs
    $jobs | Wait-Job | ForEach-Object {
        $result = Receive-Job $_
        $results[$result.Status]++
        Remove-Job $_
    }

    Write-Host "`nResults:"
    Write-Host "  Allowed: $($results.Allowed)" -ForegroundColor Green
    Write-Host "  Denied:  $($results.Denied)" -ForegroundColor Red
    Write-Host "  Errors:  $($results.Errors)" -ForegroundColor Yellow
}

# Run it
Invoke-LoadTest -Requests 500 -Concurrent 20
```

---

## Quick Reference

**Basic Check:**
```powershell
$body = '{"resource":"api.test","identifier":"user-1"}'
Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json"
```

**With Algorithm:**
```powershell
$body = '{"resource":"api.test","identifier":"user-1","algorithm":"token_bucket"}'
Invoke-RestMethod -Uri http://localhost:8081/v1/check -Method Post -Body $body -ContentType "application/json"
```

**Status Check:**
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/status/user-1:api.test"
```

**Reset:**
```powershell
Invoke-RestMethod -Uri "http://localhost:8081/v1/reset/user-1:api.test" -Method Post
```
