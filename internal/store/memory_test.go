package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_IncrWindowCountsAndDenies(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()

	for i := int64(1); i <= 3; i++ {
		res, err := ms.IncrWindow(ctx, "k", time.Minute, 1, 3, false, time.Minute)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, i, res.Current)
	}

	res, err := ms.IncrWindow(ctx, "k", time.Minute, 1, 3, false, time.Minute)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Equal(t, int64(3), res.Current, "denied increment must not be applied")
}

func TestMemoryStore_IncrWindowPeekIsReadOnly(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		res, err := ms.IncrWindow(ctx, "k", time.Minute, 0, 3, false, time.Minute)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, int64(0), res.Current)
	}
}

func TestMemoryStore_WindowShift(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()
	window := 200 * time.Millisecond

	res, err := ms.IncrWindow(ctx, "k", window, 4, 100, true, time.Minute)
	require.NoError(t, err)
	first := res.WindowStart

	// Wait until the next window: the old current count must appear as
	// the previous count.
	time.Sleep(window + 20*time.Millisecond)
	res, err = ms.IncrWindow(ctx, "k", window, 1, 100, true, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.Current)
	assert.Equal(t, int64(4), res.Previous)
	assert.Equal(t, first.Add(window), res.WindowStart)

	// Two more windows later, both counts are stale and must read zero.
	time.Sleep(2*window + 20*time.Millisecond)
	res, err = ms.IncrWindow(ctx, "k", window, 0, 100, true, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(0), res.Current)
	assert.Equal(t, int64(0), res.Previous)
}

func TestMemoryStore_TakeTokensDeniedDoesNotDoubleRefill(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()

	// Drain a 10-token bucket refilling at 10/s.
	allowed, _, err := ms.TakeTokens(ctx, "k", 10, 10, 10, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)

	// Hammer it with denied requests. Each one refills; if refill were
	// credited without advancing the timestamp, tokens would inflate far
	// beyond elapsed*rate.
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, _, err := ms.TakeTokens(ctx, "k", 10, 10, 8, time.Minute)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	_, tokens, err := ms.TakeTokens(ctx, "k", 10, 10, 0, time.Minute)
	require.NoError(t, err)
	// ~0.3s elapsed at 10 tokens/s ≈ 3 tokens; generous upper bound well
	// under what double-crediting would produce.
	assert.Less(t, tokens, 6.0)
	assert.Greater(t, tokens, 1.0)
}

func TestMemoryStore_DeleteClearsState(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()

	_, err := ms.IncrWindow(ctx, "k", time.Minute, 5, 10, false, time.Minute)
	require.NoError(t, err)
	_, _, err = ms.TakeTokens(ctx, "k", 10, 1, 5, time.Minute)
	require.NoError(t, err)

	require.NoError(t, ms.Delete(ctx, "k"))

	res, err := ms.IncrWindow(ctx, "k", time.Minute, 0, 10, false, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(0), res.Current)

	_, tokens, err := ms.TakeTokens(ctx, "k", 10, 1, 0, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 10.0, tokens, "deleted bucket must reinitialize full")
}

func TestMemoryStore_SweepEvictsOnlyExpired(t *testing.T) {
	ms := NewMemoryStore()
	defer ms.Close()
	ctx := context.Background()

	_, err := ms.IncrWindow(ctx, "expired", time.Minute, 1, 10, false, 10*time.Millisecond)
	require.NoError(t, err)
	_, err = ms.IncrWindow(ctx, "live", time.Minute, 1, 10, false, time.Hour)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	ms.sweep(time.Now().UnixMicro())

	ms.mu.RLock()
	_, expiredExists := ms.windows["expired"]
	_, liveExists := ms.windows["live"]
	ms.mu.RUnlock()
	assert.False(t, expiredExists)
	assert.True(t, liveExists)
}

func TestMemoryStore_PingAndClose(t *testing.T) {
	ms := NewMemoryStore()
	assert.NoError(t, ms.Ping(context.Background()))
	assert.NoError(t, ms.Close())
	assert.NoError(t, ms.Close(), "Close must be idempotent")
}
