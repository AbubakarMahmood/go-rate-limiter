package limiter

import "time"

// RateLimiter is the primary interface for rate limiting operations
type RateLimiter interface {
	// Allow checks if a single request is allowed for the given key
	Allow(key string) (bool, *LimitInfo, error)

	// AllowN checks if N requests are allowed for the given key
	AllowN(key string, n int) (bool, *LimitInfo, error)

	// Reset resets the rate limit for the given key
	Reset(key string) error
}

// LimitInfo provides detailed information about rate limit status
type LimitInfo struct {
	Limit      int            // Maximum number of requests allowed
	Remaining  int            // Number of requests remaining
	ResetAt    time.Time      // Time when the limit resets
	RetryAfter *time.Duration // Duration to wait before retrying (if denied)
}

// Config represents rate limiter configuration
type Config struct {
	Algorithm string        // Algorithm to use: token_bucket, sliding_window, fixed_window
	Limit     int           // Maximum number of requests
	Window    time.Duration // Time window for the limit
	Burst     int           // Burst capacity (for token bucket)
}

// Window represents a time window with request count
type Window struct {
	Timestamp time.Time
	Count     int64
}

// Store abstracts the persistence layer (Redis, in-memory, etc.)
type Store interface {
	// Increment increments the counter for a key at a specific window
	Increment(key string, window time.Time) (int64, error)

	// GetWindows returns all windows for a key within a time range
	GetWindows(key string, from, to time.Time) ([]Window, error)

	// SetTokens sets the token count and last refill time for token bucket
	SetTokens(key string, tokens float64, lastRefill time.Time) error

	// GetTokens gets the token count and last refill time for token bucket
	GetTokens(key string) (tokens float64, lastRefill time.Time, err error)

	// Delete removes all data for a key
	Delete(key string) error

	// Close closes the store connection
	Close() error
}
