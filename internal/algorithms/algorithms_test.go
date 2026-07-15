package algorithms_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemory(t *testing.T) *store.MemoryStore {
	t.Helper()
	s := store.NewMemoryStore()
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestTokenBucket_AllowUpToCapacity(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		res, err := tb.Allow(ctx, "k")
		require.NoError(t, err)
		assert.True(t, res.Allowed, "request %d", i+1)
		assert.Equal(t, 10, res.Limit)
		assert.Equal(t, 9-i, res.Remaining)
	}

	res, err := tb.Allow(ctx, "k")
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)
	assert.Greater(t, res.RetryAfter, time.Duration(0))
}

func TestTokenBucket_Refill(t *testing.T) {
	// 10 tokens per second.
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Second, Burst: 10})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := tb.Allow(ctx, "k")
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	res, err := tb.Allow(ctx, "k")
	require.NoError(t, err)
	assert.True(t, res.Allowed, "should be allowed after refill")
	assert.Greater(t, res.Remaining, 0)
}

func TestTokenBucket_SubSecondRetryAfter(t *testing.T) {
	// The refill rate is 10/s, so an empty bucket is one token short for
	// only ~100ms. A whole-second RetryAfter here would indicate the
	// duration math truncates.
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Second, Burst: 10})
	ctx := context.Background()

	_, err := tb.AllowN(ctx, "k", 10)
	require.NoError(t, err)

	res, err := tb.Allow(ctx, "k")
	require.NoError(t, err)
	require.False(t, res.Allowed)
	assert.Greater(t, res.RetryAfter, time.Duration(0))
	assert.Less(t, res.RetryAfter, 500*time.Millisecond)
}

func TestTokenBucket_AllowNAllOrNothing(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	res, err := tb.AllowN(ctx, "k", 5)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 5, res.Remaining)

	// A denied request must not consume anything.
	res, err = tb.AllowN(ctx, "k", 6)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, 5, res.Remaining)

	res, err = tb.AllowN(ctx, "k", 5)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)
}

func TestTokenBucket_InvalidCounts(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	_, err := tb.AllowN(ctx, "k", -1)
	assert.ErrorIs(t, err, limiter.ErrInvalidCount)

	_, err = tb.AllowN(ctx, "k", 11)
	assert.ErrorIs(t, err, limiter.ErrExceedsLimit)
}

func TestTokenBucket_BurstDefaultsToLimit(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 7, Window: time.Hour})
	ctx := context.Background()

	res, err := tb.Peek(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 7, res.Limit)
	assert.Equal(t, 7, res.Remaining)
}

func TestTokenBucket_PeekDoesNotConsume(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		res, err := tb.Peek(ctx, "k")
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, 10, res.Remaining)
	}

	res, err := tb.Allow(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 9, res.Remaining)
}

func TestTokenBucket_Reset(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	_, err := tb.AllowN(ctx, "k", 10)
	require.NoError(t, err)
	require.NoError(t, tb.Reset(ctx, "k"))

	res, err := tb.Allow(ctx, "k")
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 9, res.Remaining)
}

func TestSlidingWindow_AllowUpToLimit(t *testing.T) {
	swc := algorithms.NewSlidingWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Minute})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		res, err := swc.Allow(ctx, "k")
		require.NoError(t, err)
		assert.True(t, res.Allowed, "request %d", i+1)
	}

	res, err := swc.Allow(ctx, "k")
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)
	assert.Greater(t, res.RetryAfter, time.Duration(0))
}

func TestSlidingWindow_AllowNConsumesN(t *testing.T) {
	swc := algorithms.NewSlidingWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Minute})
	ctx := context.Background()

	res, err := swc.AllowN(ctx, "k", 4)
	require.NoError(t, err)
	assert.True(t, res.Allowed)

	res, err = swc.AllowN(ctx, "k", 4)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 2, res.Remaining)

	res, err = swc.AllowN(ctx, "k", 3)
	require.NoError(t, err)
	assert.False(t, res.Allowed, "9th-11th permits must be denied")

	res, err = swc.AllowN(ctx, "k", 2)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)
}

func TestSlidingWindow_SlidesOpenOverTime(t *testing.T) {
	swc := algorithms.NewSlidingWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Second})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := swc.Allow(ctx, "k")
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	res, err := swc.Allow(ctx, "k")
	require.NoError(t, err)
	assert.True(t, res.Allowed)
}

func TestFixedWindow_AllowUpToLimit(t *testing.T) {
	fwc := algorithms.NewFixedWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Minute})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		res, err := fwc.Allow(ctx, "k")
		require.NoError(t, err)
		assert.True(t, res.Allowed, "request %d", i+1)
		assert.Equal(t, 9-i, res.Remaining)
	}

	res, err := fwc.Allow(ctx, "k")
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Greater(t, res.RetryAfter, time.Duration(0))
}

func TestFixedWindow_AllowNConsumesN(t *testing.T) {
	fwc := algorithms.NewFixedWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Minute})
	ctx := context.Background()

	res, err := fwc.AllowN(ctx, "k", 7)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 3, res.Remaining)

	// All-or-nothing: 4 > 3 remaining, and the denial must not consume.
	res, err = fwc.AllowN(ctx, "k", 4)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, 3, res.Remaining)

	res, err = fwc.AllowN(ctx, "k", 3)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 0, res.Remaining)
}

func TestFixedWindow_WindowRollsOver(t *testing.T) {
	fwc := algorithms.NewFixedWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: 500 * time.Millisecond})
	ctx := context.Background()

	_, err := fwc.AllowN(ctx, "k", 10)
	require.NoError(t, err)

	res, err := fwc.Allow(ctx, "k")
	require.NoError(t, err)
	require.False(t, res.Allowed)

	time.Sleep(600 * time.Millisecond)

	res, err = fwc.Allow(ctx, "k")
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, 9, res.Remaining)
}

func TestFixedWindow_PeekDoesNotConsume(t *testing.T) {
	fwc := algorithms.NewFixedWindowCounter(newMemory(t), limiter.Config{Limit: 10, Window: time.Minute})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		res, err := fwc.Peek(ctx, "k")
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, 10, res.Remaining)
	}

	res, err := fwc.Allow(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 9, res.Remaining)
}

// TestAlgorithmsDoNotShareState guards against different algorithms reading
// each other's counters for the same client key.
func TestAlgorithmsDoNotShareState(t *testing.T) {
	s := newMemory(t)
	cfg := limiter.Config{Limit: 5, Window: time.Minute, Burst: 5}
	ctx := context.Background()

	fixed := algorithms.NewFixedWindowCounter(s, cfg)
	sliding := algorithms.NewSlidingWindowCounter(s, cfg)
	bucket := algorithms.NewTokenBucket(s, cfg)

	_, err := fixed.AllowN(ctx, "k", 5)
	require.NoError(t, err)

	res, err := sliding.AllowN(ctx, "k", 5)
	require.NoError(t, err)
	assert.True(t, res.Allowed, "sliding window must not see fixed window's count")

	res, err = bucket.AllowN(ctx, "k", 5)
	require.NoError(t, err)
	assert.True(t, res.Allowed, "token bucket must not see window counts")
}

// TestConcurrentAdmissionIsExact is the atomicity contract: under
// contention, exactly limit permits may be granted, never more.
func TestConcurrentAdmissionIsExact(t *testing.T) {
	s := newMemory(t)
	ctx := context.Background()

	limiters := map[string]limiter.RateLimiter{
		// The hour-long window makes token refill negligible during the test.
		"token_bucket":   algorithms.NewTokenBucket(s, limiter.Config{Limit: 100, Window: time.Hour, Burst: 100}),
		"sliding_window": algorithms.NewSlidingWindowCounter(s, limiter.Config{Limit: 100, Window: time.Hour}),
		"fixed_window":   algorithms.NewFixedWindowCounter(s, limiter.Config{Limit: 100, Window: time.Hour}),
	}

	for name, lim := range limiters {
		t.Run(name, func(t *testing.T) {
			var wg sync.WaitGroup
			results := make(chan bool, 300)
			for i := 0; i < 300; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					res, err := lim.Allow(ctx, "hot-key-"+name)
					if err == nil {
						results <- res.Allowed
					}
				}()
			}
			wg.Wait()
			close(results)

			allowed := 0
			for a := range results {
				if a {
					allowed++
				}
			}
			assert.Equal(t, 100, allowed)
		})
	}
}

func TestMultipleKeysAreIndependent(t *testing.T) {
	tb := algorithms.NewTokenBucket(newMemory(t), limiter.Config{Limit: 10, Window: time.Hour, Burst: 10})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		res1, err := tb.Allow(ctx, "key1")
		require.NoError(t, err)
		res2, err := tb.Allow(ctx, "key2")
		require.NoError(t, err)
		assert.True(t, res1.Allowed)
		assert.True(t, res2.Allowed)
	}

	res1, _ := tb.Allow(ctx, "key1")
	res2, _ := tb.Allow(ctx, "key2")
	assert.False(t, res1.Allowed)
	assert.False(t, res2.Allowed)
}
