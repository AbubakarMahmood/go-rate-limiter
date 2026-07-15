// Package algorithms implements the rate-limiting algorithms on top of the
// atomic operations exposed by limiter.Store. The algorithms hold no mutable
// state and no locks of their own: all synchronization lives in the store,
// which is what makes the same code correct for both the in-memory backend
// and a Redis backend shared by many application instances.
package algorithms

import (
	"context"
	"fmt"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
)

// FixedWindowCounter divides time into fixed windows and counts requests in
// each. It is the cheapest algorithm — one counter per key — at the cost of
// admitting up to 2x the limit across a window boundary.
type FixedWindowCounter struct {
	store  limiter.Store
	limit  int
	window time.Duration
	ttl    time.Duration
}

// NewFixedWindowCounter creates a fixed-window limiter allowing
// config.Limit requests per config.Window.
func NewFixedWindowCounter(store limiter.Store, config limiter.Config) *FixedWindowCounter {
	return &FixedWindowCounter{
		store:  store,
		limit:  config.Limit,
		window: config.Window,
		// Once two windows have passed, expired state is indistinguishable
		// from a zero counter, so it is safe to evict.
		ttl: 2*config.Window + time.Second,
	}
}

// Allow implements limiter.RateLimiter.
func (f *FixedWindowCounter) Allow(ctx context.Context, key string) (*limiter.Result, error) {
	return f.AllowN(ctx, key, 1)
}

// AllowN implements limiter.RateLimiter.
func (f *FixedWindowCounter) AllowN(ctx context.Context, key string, n int) (*limiter.Result, error) {
	if n < 0 {
		return nil, limiter.ErrInvalidCount
	}
	if n > f.limit {
		return nil, limiter.ErrExceedsLimit
	}

	w, err := f.store.IncrWindow(ctx, "fw:"+key, f.window, int64(n), int64(f.limit), false, f.ttl)
	if err != nil {
		return nil, fmt.Errorf("fixed window: %w", err)
	}
	return f.result(w), nil
}

// Peek implements limiter.RateLimiter.
func (f *FixedWindowCounter) Peek(ctx context.Context, key string) (*limiter.Result, error) {
	w, err := f.store.IncrWindow(ctx, "fw:"+key, f.window, 0, int64(f.limit), false, f.ttl)
	if err != nil {
		return nil, fmt.Errorf("fixed window: %w", err)
	}

	r := f.result(w)
	r.Allowed = r.Remaining >= 1
	if !r.Allowed {
		r.RetryAfter = r.ResetAt.Sub(w.Now)
	}
	return r, nil
}

// Reset implements limiter.RateLimiter.
func (f *FixedWindowCounter) Reset(ctx context.Context, key string) error {
	return f.store.Delete(ctx, "fw:"+key)
}

func (f *FixedWindowCounter) result(w *limiter.WindowResult) *limiter.Result {
	remaining := f.limit - int(w.Current)
	if remaining < 0 {
		remaining = 0
	}

	resetAt := w.WindowStart.Add(f.window)
	r := &limiter.Result{
		Allowed:   w.Allowed,
		Limit:     f.limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}
	if !w.Allowed {
		r.RetryAfter = resetAt.Sub(w.Now)
	}
	return r
}
