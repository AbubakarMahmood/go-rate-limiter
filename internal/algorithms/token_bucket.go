package algorithms

import (
	"context"
	"fmt"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
)

// TokenBucket refills a bucket of capacity tokens at a constant rate; each
// request consumes tokens. It shapes traffic to a smooth average rate while
// letting clients spend accumulated tokens in bursts up to the capacity.
type TokenBucket struct {
	store        limiter.Store
	capacity     int
	refillPerSec float64
	ttl          time.Duration
}

// NewTokenBucket creates a token-bucket limiter that refills config.Limit
// tokens per config.Window, holding at most config.Burst tokens
// (config.Limit when Burst is zero).
func NewTokenBucket(store limiter.Store, config limiter.Config) *TokenBucket {
	capacity := config.Burst
	if capacity <= 0 {
		capacity = config.Limit
	}
	refillPerSec := float64(config.Limit) / config.Window.Seconds()

	return &TokenBucket{
		store:        store,
		capacity:     capacity,
		refillPerSec: refillPerSec,
		// Give idle state exactly as long as a full refill takes, plus
		// slack: an evicted bucket re-initializes full, so expiring any
		// earlier would hand out tokens ahead of schedule.
		ttl: durationFromSeconds(float64(capacity)/refillPerSec) + time.Second,
	}
}

// Allow implements limiter.RateLimiter.
func (tb *TokenBucket) Allow(ctx context.Context, key string) (*limiter.Result, error) {
	return tb.AllowN(ctx, key, 1)
}

// AllowN implements limiter.RateLimiter.
func (tb *TokenBucket) AllowN(ctx context.Context, key string, n int) (*limiter.Result, error) {
	if n < 0 {
		return nil, limiter.ErrInvalidCount
	}
	if n > tb.capacity {
		return nil, limiter.ErrExceedsLimit
	}

	allowed, tokens, err := tb.store.TakeTokens(ctx, "tb:"+key, float64(tb.capacity), tb.refillPerSec, float64(n), tb.ttl)
	if err != nil {
		return nil, fmt.Errorf("token bucket: %w", err)
	}
	return tb.result(allowed, tokens, n), nil
}

// Peek implements limiter.RateLimiter.
func (tb *TokenBucket) Peek(ctx context.Context, key string) (*limiter.Result, error) {
	_, tokens, err := tb.store.TakeTokens(ctx, "tb:"+key, float64(tb.capacity), tb.refillPerSec, 0, tb.ttl)
	if err != nil {
		return nil, fmt.Errorf("token bucket: %w", err)
	}
	return tb.result(tokens >= 1, tokens, 1), nil
}

// Reset implements limiter.RateLimiter.
func (tb *TokenBucket) Reset(ctx context.Context, key string) error {
	return tb.store.Delete(ctx, "tb:"+key)
}

func (tb *TokenBucket) result(allowed bool, tokens float64, n int) *limiter.Result {
	r := &limiter.Result{
		Allowed:   allowed,
		Limit:     tb.capacity,
		Remaining: int(tokens),
		// ResetAt reports when the bucket will be full again at the
		// current refill rate.
		ResetAt: time.Now().Add(durationFromSeconds((float64(tb.capacity) - tokens) / tb.refillPerSec)),
	}
	if !allowed {
		r.RetryAfter = durationFromSeconds((float64(n) - tokens) / tb.refillPerSec)
	}
	return r
}

// durationFromSeconds converts fractional seconds without the truncation
// that time.Duration(int) * time.Second would introduce.
func durationFromSeconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
