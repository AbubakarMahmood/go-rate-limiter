package store

import (
	"sync"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
)

// MemoryStore implements an in-memory store for rate limiting
// Good for single-instance deployments and testing
// Uses sync.Map for concurrent access
type MemoryStore struct {
	// counters stores window-based counters (for fixed/sliding window)
	counters sync.Map // map[string]map[time.Time]int64

	// tokens stores token bucket state
	tokens sync.Map // map[string]*tokenState

	// mu protects cleanup operations
	mu sync.RWMutex
}

type tokenState struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.RWMutex
}

type windowCounts struct {
	data map[time.Time]int64
	mu   sync.RWMutex
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	ms := &MemoryStore{}
	// Start background cleanup goroutine
	go ms.cleanup()
	return ms
}

// Increment increments the counter for a key at a specific window
func (ms *MemoryStore) Increment(key string, window time.Time) (int64, error) {
	// Load or create window counts for this key
	val, _ := ms.counters.LoadOrStore(key, &windowCounts{
		data: make(map[time.Time]int64),
	})

	wc := val.(*windowCounts)
	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.data[window]++
	return wc.data[window], nil
}

// GetWindows returns all windows for a key within a time range
func (ms *MemoryStore) GetWindows(key string, from, to time.Time) ([]limiter.Window, error) {
	val, ok := ms.counters.Load(key)
	if !ok {
		return []limiter.Window{}, nil
	}

	wc := val.(*windowCounts)
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	windows := make([]limiter.Window, 0)
	for t, count := range wc.data {
		if (t.Equal(from) || t.After(from)) && (t.Equal(to) || t.Before(to)) {
			windows = append(windows, limiter.Window{
				Timestamp: t,
				Count:     count,
			})
		}
	}

	return windows, nil
}

// SetTokens sets the token count and last refill time for token bucket
func (ms *MemoryStore) SetTokens(key string, tokens float64, lastRefill time.Time) error {
	val, _ := ms.tokens.LoadOrStore(key, &tokenState{})
	ts := val.(*tokenState)

	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.tokens = tokens
	ts.lastRefill = lastRefill
	return nil
}

// GetTokens gets the token count and last refill time for token bucket
func (ms *MemoryStore) GetTokens(key string) (tokens float64, lastRefill time.Time, err error) {
	val, ok := ms.tokens.Load(key)
	if !ok {
		return 0, time.Time{}, nil
	}

	ts := val.(*tokenState)
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.tokens, ts.lastRefill, nil
}

// Delete removes all data for a key
func (ms *MemoryStore) Delete(key string) error {
	ms.counters.Delete(key)
	ms.tokens.Delete(key)
	return nil
}

// Close closes the store (no-op for memory store)
func (ms *MemoryStore) Close() error {
	return nil
}

// cleanup periodically removes old window data to prevent memory leaks
func (ms *MemoryStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Remove windows older than 24 hours
		cutoff := time.Now().Add(-24 * time.Hour)

		ms.counters.Range(func(key, val interface{}) bool {
			wc := val.(*windowCounts)
			wc.mu.Lock()
			for t := range wc.data {
				if t.Before(cutoff) {
					delete(wc.data, t)
				}
			}
			wc.mu.Unlock()
			return true
		})
	}
}
