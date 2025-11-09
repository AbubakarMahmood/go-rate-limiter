package benchmark

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
)

// Benchmark Token Bucket algorithm
func BenchmarkTokenBucket(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
		Burst:  1000000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			tb.Allow(key)
			i++
		}
	})
}

// Benchmark Sliding Window Counter algorithm
func BenchmarkSlidingWindowCounter(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	swc := algorithms.NewSlidingWindowCounter(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			swc.Allow(key)
			i++
		}
	})
}

// Benchmark Fixed Window Counter algorithm
func BenchmarkFixedWindowCounter(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	fwc := algorithms.NewFixedWindowCounter(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			fwc.Allow(key)
			i++
		}
	})
}

// Benchmark concurrent access with single key
func BenchmarkConcurrentSingleKey(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
		Burst:  1000000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tb.Allow("single-key")
		}
	})
}

// Benchmark concurrent access with multiple keys
func BenchmarkConcurrentMultipleKeys(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
		Burst:  1000000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			tb.Allow(key)
			i++
		}
	})
}

// Benchmark AllowN with varying sizes
func BenchmarkAllowN(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  1000000,
		Window: 1 * time.Second,
		Burst:  1000000,
	})

	sizes := []int{1, 10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("N=%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key-%d", i%100)
				tb.AllowN(key, size)
			}
		})
	}
}

// Benchmark memory store operations
func BenchmarkMemoryStoreIncrement(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			s.Increment(key, time.Now().Truncate(time.Second))
			i++
		}
	})
}

func BenchmarkMemoryStoreGetWindows(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	// Populate store
	now := time.Now()
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		s.Increment(key, now.Truncate(time.Second))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			s.GetWindows(key, now.Add(-time.Minute), now)
			i++
		}
	})
}

func BenchmarkMemoryStoreSetGetTokens(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			s.SetTokens(key, 100.0, time.Now())
			s.GetTokens(key)
			i++
		}
	})
}

// Benchmark realistic workload
func BenchmarkRealisticWorkload(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	// Create multiple rate limiters
	limiters := []limiter.RateLimiter{
		algorithms.NewTokenBucket(s, limiter.Config{
			Limit:  1000,
			Window: 1 * time.Second,
			Burst:  1200,
		}),
		algorithms.NewSlidingWindowCounter(s, limiter.Config{
			Limit:  1000,
			Window: 1 * time.Second,
		}),
		algorithms.NewFixedWindowCounter(s, limiter.Config{
			Limit:  1000,
			Window: 1 * time.Second,
		}),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Simulate different users and resources
			user := fmt.Sprintf("user-%d", i%100)
			resource := fmt.Sprintf("api.%s", []string{"users", "posts", "comments"}[i%3])
			key := user + ":" + resource

			// Use different algorithms
			limiterIndex := i % len(limiters)
			limiters[limiterIndex].Allow(key)
			i++
		}
	})
}

// Benchmark throughput test
func BenchmarkThroughput(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  100000,
		Window: 1 * time.Second,
		Burst:  100000,
	})

	// Test with different concurrency levels
	concurrencyLevels := []int{1, 10, 100, 1000}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency=%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)
			b.ResetTimer()

			var counter int64
			var mu sync.Mutex

			b.RunParallel(func(pb *testing.PB) {
				localCount := 0
				for pb.Next() {
					tb.Allow("benchmark-key")
					localCount++
				}
				mu.Lock()
				counter += int64(localCount)
				mu.Unlock()
			})

			b.ReportMetric(float64(counter)/b.Elapsed().Seconds(), "ops/sec")
		})
	}
}

// Benchmark latency percentiles
func BenchmarkLatencyPercentiles(b *testing.B) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  100000,
		Window: 1 * time.Second,
		Burst:  100000,
	})

	latencies := make([]time.Duration, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		tb.Allow(fmt.Sprintf("key-%d", i%100))
		latencies[i] = time.Since(start)
	}

	// This is just for demonstration - actual percentile calculation would need sorting
	b.ReportMetric(float64(latencies[0].Nanoseconds()), "ns/op_sample")
}
