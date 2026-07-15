package store

// Integration tests against a real Redis. They run when REDIS_ADDR is set
// (as in CI, where a Redis service container is provided) and skip
// otherwise, so the default `go test ./...` needs no infrastructure.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedis(t *testing.T) *RedisStore {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping Redis integration tests")
	}
	rs, err := NewRedisStore(RedisConfig{Addresses: []string{addr}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rs.Close() })
	return rs
}

// uniqueKey isolates test runs from leftover state in a shared Redis.
func uniqueKey(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func TestRedisStore_IncrWindowCountsAndDenies(t *testing.T) {
	rs := newRedis(t)
	ctx := context.Background()
	key := uniqueKey("win")

	for i := int64(1); i <= 3; i++ {
		res, err := rs.IncrWindow(ctx, key, time.Minute, 1, 3, false, time.Minute)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, i, res.Current)
	}

	res, err := rs.IncrWindow(ctx, key, time.Minute, 1, 3, false, time.Minute)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, int64(3), res.Current, "denied increment must not be applied")

	// Peek stays read-only.
	res, err = rs.IncrWindow(ctx, key, time.Minute, 0, 3, false, time.Minute)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
	assert.Equal(t, int64(3), res.Current)
}

func TestRedisStore_ConcurrentAdmissionIsExact(t *testing.T) {
	rs := newRedis(t)
	ctx := context.Background()

	t.Run("window", func(t *testing.T) {
		key := uniqueKey("hot-win")
		var wg sync.WaitGroup
		results := make(chan bool, 300)
		for i := 0; i < 300; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := rs.IncrWindow(ctx, key, time.Hour, 1, 100, false, time.Hour)
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

	t.Run("tokens", func(t *testing.T) {
		key := uniqueKey("hot-tb")
		var wg sync.WaitGroup
		results := make(chan bool, 300)
		for i := 0; i < 300; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Refill over an hour is negligible during the test.
				allowed, _, err := rs.TakeTokens(ctx, key, 100, 100.0/3600, 1, time.Hour)
				if err == nil {
					results <- allowed
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

func TestRedisStore_TakeTokensRefills(t *testing.T) {
	rs := newRedis(t)
	ctx := context.Background()
	key := uniqueKey("tb")

	allowed, tokens, err := rs.TakeTokens(ctx, key, 10, 10, 10, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)
	assert.InDelta(t, 0, tokens, 0.1)

	time.Sleep(500 * time.Millisecond)

	allowed, tokens, err = rs.TakeTokens(ctx, key, 10, 10, 3, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed, "≈5 tokens should have refilled, got %f", tokens)
}

func TestRedisStore_StateExpires(t *testing.T) {
	rs := newRedis(t)
	ctx := context.Background()
	key := uniqueKey("ttl")

	_, err := rs.IncrWindow(ctx, key, time.Minute, 1, 10, false, time.Minute)
	require.NoError(t, err)

	ttl, err := rs.client.TTL(ctx, "rl:"+key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "counter keys must carry a TTL")
	assert.LessOrEqual(t, ttl, time.Minute+time.Second)
}

func TestRedisStore_DeleteClearsState(t *testing.T) {
	rs := newRedis(t)
	ctx := context.Background()
	key := uniqueKey("del")

	_, _, err := rs.TakeTokens(ctx, key, 10, 1, 10, time.Hour)
	require.NoError(t, err)
	require.NoError(t, rs.Delete(ctx, key))

	_, tokens, err := rs.TakeTokens(ctx, key, 10, 1, 0, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 10.0, tokens, "deleted bucket must reinitialize full")
}
