package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
)

// TokenBucket implements the token bucket rate limiting algorithm
// Tokens are added at a constant rate, and each request consumes one token
// Provides smooth rate limiting with burst handling
type TokenBucket struct {
	store      limiter.Store
	capacity   int           // Maximum tokens in bucket
	refillRate float64       // Tokens added per second
	window     time.Duration // Not used in token bucket but kept for interface consistency
	mu         sync.RWMutex  // Protects in-memory operations
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(store limiter.Store, config limiter.Config) *TokenBucket {
	capacity := config.Burst
	if capacity == 0 {
		capacity = config.Limit
	}

	// Calculate refill rate: tokens per second
	refillRate := float64(config.Limit) / config.Window.Seconds()

	return &TokenBucket{
		store:      store,
		capacity:   capacity,
		refillRate: refillRate,
		window:     config.Window,
	}
}

// Allow checks if a single request is allowed
func (tb *TokenBucket) Allow(key string) (bool, *limiter.LimitInfo, error) {
	return tb.AllowN(key, 1)
}

// AllowN checks if N requests are allowed
func (tb *TokenBucket) AllowN(key string, n int) (bool, *limiter.LimitInfo, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Get current tokens and last refill time
	tokens, lastRefill, err := tb.store.GetTokens(key)
	if err != nil {
		// First request - initialize with full bucket
		tokens = float64(tb.capacity)
		lastRefill = now
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(lastRefill).Seconds()
	tokens += elapsed * tb.refillRate

	// Cap at capacity
	if tokens > float64(tb.capacity) {
		tokens = float64(tb.capacity)
	}

	// Check if enough tokens available
	allowed := tokens >= float64(n)
	remaining := int(tokens)

	if allowed {
		tokens -= float64(n)
		remaining = int(tokens)
	}

	// Save updated state
	if err := tb.store.SetTokens(key, tokens, now); err != nil {
		return false, nil, fmt.Errorf("failed to update tokens: %w", err)
	}

	// Calculate reset time (when bucket will be full again)
	tokensNeeded := float64(tb.capacity) - tokens
	resetDuration := time.Duration(tokensNeeded/tb.refillRate) * time.Second
	resetAt := now.Add(resetDuration)

	info := &limiter.LimitInfo{
		Limit:     tb.capacity,
		Remaining: remaining,
		ResetAt:   resetAt,
	}

	// If denied, calculate retry after
	if !allowed {
		tokensNeeded := float64(n) - tokens
		retryAfter := time.Duration(tokensNeeded/tb.refillRate) * time.Second
		info.RetryAfter = &retryAfter
	}

	return allowed, info, nil
}

// Reset resets the rate limit for a key
func (tb *TokenBucket) Reset(key string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.store.Delete(key)
}
