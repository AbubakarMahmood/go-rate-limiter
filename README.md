# Go-Rate-Limiter-Prompt
I need you to create a CLAUDE.md file for building a high-performance, distributed rate limiter service in Go that can handle millions of requests per second.

Project Overview:
Build a production-grade rate limiting microservice that implements multiple algorithms and can be used as a sidecar or standalone service. This demonstrates systems programming, concurrency, and performance optimization skills.

Tech Stack:
- Go 1.21+
- Redis for distributed state
- PostgreSQL for configuration
- Gin or Fiber for HTTP framework
- Prometheus for metrics
- Grafana for visualization (docker-compose)
- Vegeta for load testing

Core Algorithms to Implement:

1. Token Bucket:
   - Configurable capacity and refill rate
   - Smooth traffic shaping
   - Good for steady rate limiting

2. Sliding Window Log:
   - Precise rate limiting
   - Higher memory usage
   - Best accuracy

3. Sliding Window Counter:
   - Hybrid approach
   - Memory efficient
   - Good accuracy

4. Fixed Window Counter:
   - Simple, fast
   - Allows bursts at window boundaries
   - Lowest memory usage

Architecture Components:
```go
// Core interfaces
type RateLimiter interface {
    Allow(key string) (bool, *LimitInfo, error)
    AllowN(key string, n int) (bool, *LimitInfo, error)
    Reset(key string) error
}

type LimitInfo struct {
    Limit     int
    Remaining int
    ResetAt   time.Time
    RetryAfter *time.Duration
}

type Store interface {
    Increment(key string, window time.Time) (int64, error)
    GetWindows(key string, from, to time.Time) ([]Window, error)
    SetTokens(key string, tokens float64, lastRefill time.Time) error
}
```

Features:

1. Multi-tenant Support:
   - API key based identification
   - Per-tenant configurations
   - Dynamic limit updates

2. Flexible Configuration:
   - Per-endpoint limits
   - User tier-based limits
   - Geographic-based limits
   - Time-based limits (peak hours)

3. HTTP API:
POST   /v1/check      # Check if request allowed
GET    /v1/status/:key # Get current limit status
POST   /v1/reset/:key  # Admin: reset limits
GET    /v1/metrics     # Prometheus metrics
GET    /health         # Health check
PUT    /v1/config     # Update limits dynamically

4. Response Headers:
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1609459200
Retry-After: 30

Performance Requirements:
- Sub-millisecond latency (p99 < 1ms)
- 100,000+ requests per second per instance
- Horizontal scaling with Redis
- Minimal memory footprint
- Zero allocation in hot paths

Project Structure:
rate-limiter/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── algorithms/
│   │   ├── token_bucket.go
│   │   ├── sliding_window.go
│   │   └── fixed_window.go
│   ├── store/
│   │   ├── redis.go
│   │   └── memory.go
│   ├── config/
│   │   └── config.go
│   ├── handlers/
│   │   └── rate_limit.go
│   └── metrics/
│       └── prometheus.go
├── pkg/
│   └── limiter/
│       └── client.go
├── scripts/
│   ├── load-test.sh
│   └── benchmark.go
├── docker/
│   └── docker-compose.yml
└── tests/
├── unit/
├── integration/
└── benchmark/

Implementation Priorities:

1. Phase 1: Core Algorithm
   - Start with token bucket (simpler)
   - In-memory store first
   - Basic HTTP endpoint
   - Unit tests

2. Phase 2: Redis Integration
   - Implement Redis store
   - Lua scripts for atomicity
   - Connection pooling
   - Failover handling

3. Phase 3: Performance
   - Benchmark with pprof
   - Optimize allocations
   - Add caching layer
   - Implement batching

4. Phase 4: Production Features
   - Metrics and monitoring
   - Configuration hot reload
   - Admin API
   - Client SDK

Benchmarking Suite:
```go
// Benchmark different algorithms
func BenchmarkTokenBucket(b *testing.B)
func BenchmarkSlidingWindow(b *testing.B)
func BenchmarkConcurrentAccess(b *testing.B)

// Load test scenarios
- Steady load
- Burst traffic
- Distributed clients
- Configuration changes under load
```

Redis Optimization:
- Use Lua scripts for atomic operations
- Pipeline commands when possible
- Implement connection pooling
- Use Redis Cluster for scaling
- Implement circuit breaker for Redis failures

Testing Requirements:
- Unit tests for each algorithm
- Integration tests with Redis
- Benchmark tests for performance
- Chaos testing (Redis failure, network partition)
- Load testing with Vegeta showing graphs

Performance Optimizations:
- Use sync.Pool for object reuse
- Minimize allocations with arrays vs slices
- Use atomic operations where possible
- Implement efficient serialization
- Profile with pprof regularly

Common Pitfalls:
- Don't use floats for token calculations (use int64)
- Don't forget to handle Redis connection failures
- Don't block on Redis calls (use context with timeout)
- Don't ignore time synchronization issues
- Don't forget to expire old keys in Redis

Deliverables:
- Complete service with 4 algorithms
- Docker-compose setup with Redis + Grafana
- Load test results with graphs
- Benchmark comparisons
- Client SDK in Go
- Comprehensive README with architecture decisions

Create a comprehensive CLAUDE.md that guides through building this service with emphasis on performance, correctness, and production-readiness. Include detailed explanations of each algorithm, performance benchmarks, and real-world deployment considerations. Provide complete code examples for critical sections and explain Go-specific optimizations.
