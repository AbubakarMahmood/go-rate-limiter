package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
)

// SlidingWindowCounter implements sliding window counter algorithm
// Hybrid approach that combines fixed windows with weighted counting
// Provides good accuracy with better memory efficiency than sliding window log
type SlidingWindowCounter struct {
	store  limiter.Store
	limit  int
	window time.Duration
	mu     sync.RWMutex
}

// NewSlidingWindowCounter creates a new sliding window counter rate limiter
func NewSlidingWindowCounter(store limiter.Store, config limiter.Config) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		store:  store,
		limit:  config.Limit,
		window: config.Window,
	}
}

// Allow checks if a single request is allowed
func (swc *SlidingWindowCounter) Allow(key string) (bool, *limiter.LimitInfo, error) {
	return swc.AllowN(key, 1)
}

// AllowN checks if N requests are allowed
func (swc *SlidingWindowCounter) AllowN(key string, n int) (bool, *limiter.LimitInfo, error) {
	swc.mu.Lock()
	defer swc.mu.Unlock()

	now := time.Now()

	// Get current and previous window
	currentWindow := now.Truncate(swc.window)
	previousWindow := currentWindow.Add(-swc.window)

	// Get counts from both windows
	windows, err := swc.store.GetWindows(key, previousWindow, now)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get windows: %w", err)
	}

	// Calculate weighted count
	var currentCount, previousCount int64
	for _, w := range windows {
		if w.Timestamp.Equal(currentWindow) {
			currentCount = w.Count
		} else if w.Timestamp.Equal(previousWindow) {
			previousCount = w.Count
		}
	}

	// Calculate the weight of the previous window
	// This gives us a smooth sliding window effect
	elapsedInCurrentWindow := now.Sub(currentWindow)
	weight := 1.0 - (float64(elapsedInCurrentWindow) / float64(swc.window))

	// Weighted count = current window + (previous window * weight)
	weightedCount := float64(currentCount) + (float64(previousCount) * weight)

	// Check if request allowed
	allowed := weightedCount+float64(n) <= float64(swc.limit)

	if allowed {
		// Increment current window
		newCount, err := swc.store.Increment(key, currentWindow)
		if err != nil {
			return false, nil, fmt.Errorf("failed to increment: %w", err)
		}
		currentCount = newCount
		// Recalculate weighted count after increment
		weightedCount = float64(currentCount) + (float64(previousCount) * weight)
	}

	remaining := int(float64(swc.limit) - weightedCount)
	if remaining < 0 {
		remaining = 0
	}

	// Reset time is at the start of next window
	resetAt := currentWindow.Add(swc.window)

	info := &limiter.LimitInfo{
		Limit:     swc.limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}

	// Calculate retry after if denied
	if !allowed {
		// Retry after the window slides enough to allow the request
		retryAfter := resetAt.Sub(now)
		info.RetryAfter = &retryAfter
	}

	return allowed, info, nil
}

// Reset resets the rate limit for a key
func (swc *SlidingWindowCounter) Reset(key string) error {
	swc.mu.Lock()
	defer swc.mu.Unlock()
	return swc.store.Delete(key)
}
