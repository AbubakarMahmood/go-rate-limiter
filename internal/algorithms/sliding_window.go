package algorithms

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
)

// SlidingWindowCounter approximates a true sliding window by weighting the
// previous fixed window by how much of it still overlaps the sliding one:
//
//	weighted = current + previous * (1 - elapsed/window)
//
// It smooths the boundary bursts fixed windows allow while storing only two
// counters per key, and is a good default for most workloads.
type SlidingWindowCounter struct {
	store  limiter.Store
	limit  int
	window time.Duration
	ttl    time.Duration
}

// NewSlidingWindowCounter creates a sliding-window limiter allowing
// config.Limit requests per config.Window.
func NewSlidingWindowCounter(store limiter.Store, config limiter.Config) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		store:  store,
		limit:  config.Limit,
		window: config.Window,
		ttl:    2*config.Window + time.Second,
	}
}

// Allow implements limiter.RateLimiter.
func (s *SlidingWindowCounter) Allow(ctx context.Context, key string) (*limiter.Result, error) {
	return s.AllowN(ctx, key, 1)
}

// AllowN implements limiter.RateLimiter.
func (s *SlidingWindowCounter) AllowN(ctx context.Context, key string, n int) (*limiter.Result, error) {
	if n < 0 {
		return nil, limiter.ErrInvalidCount
	}
	if n > s.limit {
		return nil, limiter.ErrExceedsLimit
	}

	w, err := s.store.IncrWindow(ctx, "sw:"+key, s.window, int64(n), int64(s.limit), true, s.ttl)
	if err != nil {
		return nil, fmt.Errorf("sliding window: %w", err)
	}
	return s.result(w, n), nil
}

// Peek implements limiter.RateLimiter.
func (s *SlidingWindowCounter) Peek(ctx context.Context, key string) (*limiter.Result, error) {
	w, err := s.store.IncrWindow(ctx, "sw:"+key, s.window, 0, int64(s.limit), true, s.ttl)
	if err != nil {
		return nil, fmt.Errorf("sliding window: %w", err)
	}

	r := s.result(w, 1)
	r.Allowed = r.Remaining >= 1
	if !r.Allowed {
		r.RetryAfter = s.retryAfter(w, 1)
	}
	return r, nil
}

// Reset implements limiter.RateLimiter.
func (s *SlidingWindowCounter) Reset(ctx context.Context, key string) error {
	return s.store.Delete(ctx, "sw:"+key)
}

func (s *SlidingWindowCounter) result(w *limiter.WindowResult, n int) *limiter.Result {
	weight := 1 - float64(w.Now.Sub(w.WindowStart))/float64(s.window)
	if weight < 0 {
		weight = 0
	}
	weighted := float64(w.Current) + float64(w.Previous)*weight

	// Ceil is the conservative choice: never promise a permit the weighted
	// count would deny. The epsilon keeps exact integers from rounding up.
	remaining := s.limit - int(math.Ceil(weighted-1e-9))
	if remaining < 0 {
		remaining = 0
	}

	r := &limiter.Result{
		Allowed:   w.Allowed,
		Limit:     s.limit,
		Remaining: remaining,
		ResetAt:   w.WindowStart.Add(s.window),
	}
	if !w.Allowed {
		r.RetryAfter = s.retryAfter(w, n)
	}
	return r
}

// retryAfter solves the weighted-count formula for the earliest time the
// denied request could succeed. With x the elapsed fraction of the current
// window, admission requires
//
//	current + previous*(1-x) + n <= limit
//
// which holds once x >= (current + previous + n - limit) / previous.
func (s *SlidingWindowCounter) retryAfter(w *limiter.WindowResult, n int) time.Duration {
	if w.Previous > 0 {
		frac := float64(w.Current+w.Previous+int64(n)-int64(s.limit)) / float64(w.Previous)
		if frac <= 1 {
			target := w.WindowStart.Add(time.Duration(frac * float64(s.window)))
			if d := target.Sub(w.Now); d > 0 {
				return d
			}
		}
	}
	// The previous window sliding off is not enough; the current window
	// itself must roll over before the count can drop.
	return w.WindowStart.Add(s.window).Sub(w.Now)
}
