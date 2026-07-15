package algorithms_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
)

// benchConfig keeps every request under the limit so benchmarks measure the
// admission path, not denial short-circuits.
var benchConfig = limiter.Config{Limit: 1 << 30, Window: time.Second, Burst: 1 << 30}

func benchLimiters(s limiter.Store) map[string]limiter.RateLimiter {
	return map[string]limiter.RateLimiter{
		"token_bucket":   algorithms.NewTokenBucket(s, benchConfig),
		"sliding_window": algorithms.NewSlidingWindowCounter(s, benchConfig),
		"fixed_window":   algorithms.NewFixedWindowCounter(s, benchConfig),
	}
}

// BenchmarkAllow measures parallel throughput across 100 keys, the common
// service shape: many clients, moderate per-key contention.
func BenchmarkAllow(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()
	ctx := context.Background()

	for name, lim := range benchLimiters(s) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					_, _ = lim.Allow(ctx, fmt.Sprintf("key-%d", i%100))
					i++
				}
			})
		})
	}
}

// BenchmarkAllowSingleKey measures the worst case: every request contending
// on one key's lock.
func BenchmarkAllowSingleKey(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()
	ctx := context.Background()

	for name, lim := range benchLimiters(s) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = lim.Allow(ctx, "hot-key")
				}
			})
		})
	}
}

// BenchmarkPeek measures the read-only status path.
func BenchmarkPeek(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()
	ctx := context.Background()

	for name, lim := range benchLimiters(s) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					_, _ = lim.Peek(ctx, fmt.Sprintf("key-%d", i%100))
					i++
				}
			})
		})
	}
}
