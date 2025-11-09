package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
)

// FixedWindowCounter implements fixed window counter algorithm
// Simple and fast - divides time into fixed windows
// Trade-off: allows bursts at window boundaries (2x limit possible)
// Lowest memory usage and highest performance
type FixedWindowCounter struct {
	store  limiter.Store
	limit  int
	window time.Duration
	mu     sync.RWMutex
}

// NewFixedWindowCounter creates a new fixed window counter rate limiter
func NewFixedWindowCounter(store limiter.Store, config limiter.Config) *FixedWindowCounter {
	return &FixedWindowCounter{
		store:  store,
		limit:  config.Limit,
		window: config.Window,
	}
}

// Allow checks if a single request is allowed
func (fwc *FixedWindowCounter) Allow(key string) (bool, *limiter.LimitInfo, error) {
	return fwc.AllowN(key, 1)
}

// AllowN checks if N requests are allowed
func (fwc *FixedWindowCounter) AllowN(key string, n int) (bool, *limiter.LimitInfo, error) {
	fwc.mu.Lock()
	defer fwc.mu.Unlock()

	now := time.Now()
	// Truncate to get the current window start
	currentWindow := now.Truncate(fwc.window)

	// Get current count for this window
	windows, err := fwc.store.GetWindows(key, currentWindow, now)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get windows: %w", err)
	}

	var currentCount int64
	for _, w := range windows {
		if w.Timestamp.Equal(currentWindow) {
			currentCount = w.Count
		}
	}

	// Check if request allowed
	allowed := currentCount+int64(n) <= int64(fwc.limit)

	if allowed {
		// Increment the counter
		newCount, err := fwc.store.Increment(key, currentWindow)
		if err != nil {
			return false, nil, fmt.Errorf("failed to increment: %w", err)
		}
		currentCount = newCount
	}

	remaining := fwc.limit - int(currentCount)
	if remaining < 0 {
		remaining = 0
	}

	// Reset time is at the start of next window
	resetAt := currentWindow.Add(fwc.window)

	info := &limiter.LimitInfo{
		Limit:     fwc.limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}

	// Calculate retry after if denied
	if !allowed {
		retryAfter := resetAt.Sub(now)
		info.RetryAfter = &retryAfter
	}

	return allowed, info, nil
}

// Reset resets the rate limit for a key
func (fwc *FixedWindowCounter) Reset(key string) error {
	fwc.mu.Lock()
	defer fwc.mu.Unlock()
	return fwc.store.Delete(key)
}
