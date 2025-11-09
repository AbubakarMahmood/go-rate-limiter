# Test script for Rate Limiter API
# Run this after starting the Docker stack

param(
    [string]$BaseUrl = "http://localhost:8081"
)

Write-Host "================================" -ForegroundColor Cyan
Write-Host "Rate Limiter API Test Suite" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# Test 1: Health Check
Write-Host "1. Testing Health Endpoint..." -ForegroundColor Yellow
try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/health" -Method Get
    Write-Host "   ✓ Health Status: $($health.status)" -ForegroundColor Green
    Write-Host "   Time: $($health.time)" -ForegroundColor Gray
} catch {
    Write-Host "   ✗ Health check failed: $_" -ForegroundColor Red
}
Write-Host ""

# Test 2: Token Bucket Algorithm
Write-Host "2. Testing Token Bucket Algorithm..." -ForegroundColor Yellow
$body = @{
    resource = "api.users.create"
    identifier = "user-123"
    algorithm = "token_bucket"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/check" -Method Post -Body $body -ContentType "application/json"
    Write-Host "   ✓ Request Allowed: $($response.allowed)" -ForegroundColor Green
    Write-Host "   Limit: $($response.limit)" -ForegroundColor Gray
    Write-Host "   Remaining: $($response.remaining)" -ForegroundColor Gray
    Write-Host "   Reset At: $($response.reset_at)" -ForegroundColor Gray
} catch {
    Write-Host "   ✗ Request failed: $_" -ForegroundColor Red
}
Write-Host ""

# Test 3: Sliding Window Algorithm
Write-Host "3. Testing Sliding Window Algorithm..." -ForegroundColor Yellow
$body = @{
    resource = "api.posts.create"
    identifier = "user-456"
    algorithm = "sliding_window"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/check" -Method Post -Body $body -ContentType "application/json"
    Write-Host "   ✓ Request Allowed: $($response.allowed)" -ForegroundColor Green
    Write-Host "   Limit: $($response.limit)" -ForegroundColor Gray
    Write-Host "   Remaining: $($response.remaining)" -ForegroundColor Gray
} catch {
    Write-Host "   ✗ Request failed: $_" -ForegroundColor Red
}
Write-Host ""

# Test 4: Fixed Window Algorithm
Write-Host "4. Testing Fixed Window Algorithm..." -ForegroundColor Yellow
$body = @{
    resource = "api.comments.create"
    identifier = "user-789"
    algorithm = "fixed_window"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/check" -Method Post -Body $body -ContentType "application/json"
    Write-Host "   ✓ Request Allowed: $($response.allowed)" -ForegroundColor Green
    Write-Host "   Limit: $($response.limit)" -ForegroundColor Gray
    Write-Host "   Remaining: $($response.remaining)" -ForegroundColor Gray
} catch {
    Write-Host "   ✗ Request failed: $_" -ForegroundColor Red
}
Write-Host ""

# Test 5: Rapid Fire (Testing Rate Limiting)
Write-Host "5. Testing Rate Limiting (10 rapid requests)..." -ForegroundColor Yellow
$allowedCount = 0
$deniedCount = 0

$body = @{
    resource = "api.test"
    identifier = "test-user"
    algorithm = "token_bucket"
} | ConvertTo-Json

for ($i = 1; $i -le 10; $i++) {
    try {
        $response = Invoke-RestMethod -Uri "$BaseUrl/v1/check" -Method Post -Body $body -ContentType "application/json" -ErrorAction SilentlyContinue
        if ($response.allowed) {
            $allowedCount++
            Write-Host "   Request $i : ✓ Allowed (Remaining: $($response.remaining))" -ForegroundColor Green
        } else {
            $deniedCount++
            Write-Host "   Request $i : ✗ Denied (Rate limited)" -ForegroundColor Red
        }
    } catch {
        $deniedCount++
        Write-Host "   Request $i : ✗ Denied (429 Too Many Requests)" -ForegroundColor Red
    }
    Start-Sleep -Milliseconds 50
}

Write-Host ""
Write-Host "   Summary: $allowedCount allowed, $deniedCount denied" -ForegroundColor Cyan
Write-Host ""

# Test 6: Get Status
Write-Host "6. Testing Status Endpoint..." -ForegroundColor Yellow
try {
    $status = Invoke-RestMethod -Uri "$BaseUrl/v1/status/test-user:api.test?algorithm=token_bucket" -Method Get
    Write-Host "   ✓ Status Retrieved" -ForegroundColor Green
    Write-Host "   Remaining: $($status.remaining)" -ForegroundColor Gray
} catch {
    Write-Host "   ✗ Status check failed: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "================================" -ForegroundColor Cyan
Write-Host "Testing Complete!" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Available Services:" -ForegroundColor Yellow
Write-Host "  - API:        $BaseUrl" -ForegroundColor Gray
Write-Host "  - Health:     $BaseUrl/health" -ForegroundColor Gray
Write-Host "  - Metrics:    $BaseUrl/metrics" -ForegroundColor Gray
Write-Host "  - Prometheus: http://localhost:9090" -ForegroundColor Gray
Write-Host "  - Grafana:    http://localhost:3000 (admin/admin)" -ForegroundColor Gray
Write-Host ""
