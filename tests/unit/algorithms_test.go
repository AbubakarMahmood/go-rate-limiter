package unit

import (
	"testing"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood1/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenBucket_Allow(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
		Burst:  10,
	})

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		allowed, info, err := tb.Allow("test-key")
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
		assert.Equal(t, 10, info.Limit)
		assert.Equal(t, 9-i, info.Remaining)
	}

	// 11th request should be denied
	allowed, info, err := tb.Allow("test-key")
	require.NoError(t, err)
	assert.False(t, allowed, "11th request should be denied")
	assert.Equal(t, 0, info.Remaining)
	assert.NotNil(t, info.RetryAfter)
}

func TestTokenBucket_Refill(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
		Burst:  10,
	})

	// Consume all tokens
	for i := 0; i < 10; i++ {
		tb.Allow("test-key")
	}

	// Wait for refill (10 tokens per second)
	time.Sleep(500 * time.Millisecond)

	// Should have ~5 tokens refilled
	allowed, info, err := tb.Allow("test-key")
	require.NoError(t, err)
	assert.True(t, allowed, "request should be allowed after refill")
	assert.Greater(t, info.Remaining, 0)
}

func TestTokenBucket_AllowN(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
		Burst:  10,
	})

	// Consume 5 tokens
	allowed, info, err := tb.AllowN("test-key", 5)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 5, info.Remaining)

	// Try to consume 6 tokens (should fail)
	allowed, info, err = tb.AllowN("test-key", 6)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, 5, info.Remaining)

	// Consume remaining 5 tokens (should succeed)
	allowed, info, err = tb.AllowN("test-key", 5)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 0, info.Remaining)
}

func TestTokenBucket_Reset(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
		Burst:  10,
	})

	// Consume all tokens
	for i := 0; i < 10; i++ {
		tb.Allow("test-key")
	}

	// Reset should restore full capacity
	err := tb.Reset("test-key")
	require.NoError(t, err)

	allowed, info, err := tb.Allow("test-key")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 10, info.Limit)
}

func TestSlidingWindowCounter_Allow(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	swc := algorithms.NewSlidingWindowCounter(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
	})

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		allowed, info, err := swc.Allow("test-key")
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
		assert.LessOrEqual(t, info.Remaining, 10-i)
	}

	// 11th request should be denied
	allowed, _, err := swc.Allow("test-key")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestSlidingWindowCounter_SlidingBehavior(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	swc := algorithms.NewSlidingWindowCounter(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
	})

	// Consume 5 tokens
	for i := 0; i < 5; i++ {
		swc.Allow("test-key")
	}

	// Wait for half window
	time.Sleep(500 * time.Millisecond)

	// Should be able to make more requests as window slides
	allowed, _, err := swc.Allow("test-key")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestFixedWindowCounter_Allow(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	fwc := algorithms.NewFixedWindowCounter(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
	})

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		allowed, info, err := fwc.Allow("test-key")
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
		assert.Equal(t, 10, info.Limit)
	}

	// 11th request should be denied
	allowed, _, err := fwc.Allow("test-key")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestFixedWindowCounter_WindowReset(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	fwc := algorithms.NewFixedWindowCounter(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
	})

	// Consume all tokens
	for i := 0; i < 10; i++ {
		fwc.Allow("test-key")
	}

	// Wait for window to reset
	time.Sleep(1100 * time.Millisecond)

	// Should be able to make requests again
	allowed, info, err := fwc.Allow("test-key")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 9, info.Remaining)
}

func TestConcurrentAccess(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  100,
		Window: 1 * time.Second,
		Burst:  100,
	})

	// Run 200 concurrent requests
	results := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func() {
			allowed, _, _ := tb.Allow("test-key")
			results <- allowed
		}()
	}

	// Count allowed requests
	allowedCount := 0
	for i := 0; i < 200; i++ {
		if <-results {
			allowedCount++
		}
	}

	// Should allow approximately 100 requests (with some tolerance for race conditions)
	assert.GreaterOrEqual(t, allowedCount, 95)
	assert.LessOrEqual(t, allowedCount, 105)
}

func TestMultipleKeys(t *testing.T) {
	s := store.NewMemoryStore()
	defer s.Close()

	tb := algorithms.NewTokenBucket(s, limiter.Config{
		Limit:  10,
		Window: 1 * time.Second,
		Burst:  10,
	})

	// Different keys should have independent limits
	for i := 0; i < 10; i++ {
		allowed1, _, _ := tb.Allow("key1")
		allowed2, _, _ := tb.Allow("key2")
		assert.True(t, allowed1)
		assert.True(t, allowed2)
	}

	// Both keys should be exhausted
	allowed1, _, _ := tb.Allow("key1")
	allowed2, _, _ := tb.Allow("key2")
	assert.False(t, allowed1)
	assert.False(t, allowed2)
}
