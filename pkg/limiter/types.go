// Package limiter defines the core rate-limiting contracts shared by all
// algorithm and storage implementations.
package limiter

import (
	"context"
	"errors"
	"time"
)

// ErrExceedsLimit is returned when a single request asks for more permits
// than the configured limit or bucket capacity can ever grant. Such a
// request can never succeed, so callers should treat it as a client error
// rather than a rate-limit denial.
var ErrExceedsLimit = errors.New("requested count exceeds the configured limit")

// ErrInvalidCount is returned when a caller passes a negative permit count.
var ErrInvalidCount = errors.New("count must not be negative")

// Result describes the outcome of a rate-limit decision.
type Result struct {
	Allowed    bool          // whether the request may proceed
	Limit      int           // configured maximum for the window / bucket capacity
	Remaining  int           // permits left after this decision
	ResetAt    time.Time     // when the window rolls over or the bucket refills completely
	RetryAfter time.Duration // how long to wait before retrying; 0 when allowed
}

// RateLimiter is implemented by every rate-limiting algorithm.
type RateLimiter interface {
	// Allow reports whether a single request for key may proceed,
	// consuming one permit if so.
	Allow(ctx context.Context, key string) (*Result, error)

	// AllowN reports whether n requests for key may proceed, consuming n
	// permits if so. The decision is all-or-nothing: either every permit
	// is granted or none are.
	AllowN(ctx context.Context, key string, n int) (*Result, error)

	// Peek returns the current state for key without consuming permits.
	Peek(ctx context.Context, key string) (*Result, error)

	// Reset clears all rate-limit state for key.
	Reset(ctx context.Context, key string) error
}

// Config carries the parameters every algorithm is constructed from.
type Config struct {
	Limit  int           // maximum requests per window (and the refill amount for token buckets)
	Window time.Duration // length of the window / refill period
	Burst  int           // token-bucket capacity; defaults to Limit when zero
}

// WindowResult is the outcome of an atomic window-counter operation.
type WindowResult struct {
	Allowed     bool      // whether the increment was applied (always true for peeks)
	Current     int64     // current-window count after the operation
	Previous    int64     // previous-window count
	WindowStart time.Time // start of the current window, per the store's clock
	Now         time.Time // the store's clock at decision time
}

// Store abstracts the state backend. Implementations must make each method
// atomic with respect to concurrent callers — including callers in other
// processes for shared backends such as Redis — because the correctness of
// every algorithm depends on it.
type Store interface {
	// IncrWindow atomically increments the counter for the window that
	// contains the current time by n, but only if the weighted count
	// (current + previous*weight, where weight is the still-overlapping
	// fraction of the previous window) plus n stays within limit.
	// With weightPrev=false the previous window is ignored, yielding
	// fixed-window semantics. n=0 performs a read-only peek.
	// ttl bounds how long idle state is retained.
	IncrWindow(ctx context.Context, key string, window time.Duration, n, limit int64, weightPrev bool, ttl time.Duration) (*WindowResult, error)

	// TakeTokens atomically refills the token bucket for key (up to
	// capacity, at refillPerSec tokens per second since the last refill)
	// and consumes n tokens if at least n are available. It returns the
	// token count after the operation. n=0 performs a read-only peek.
	// ttl bounds how long idle state is retained; it must be at least the
	// time a bucket takes to refill completely, so that expiry of an idle
	// key is indistinguishable from a full refill.
	TakeTokens(ctx context.Context, key string, capacity, refillPerSec, n float64, ttl time.Duration) (allowed bool, tokens float64, err error)

	// Delete removes all state for key.
	Delete(ctx context.Context, key string) error

	// Ping verifies the backend is reachable.
	Ping(ctx context.Context) error

	// Close releases resources held by the store.
	Close() error
}
