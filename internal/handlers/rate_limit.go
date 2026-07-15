// Package handlers implements the HTTP API.
package handlers

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/internal/metrics"
	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
	"github.com/gin-gonic/gin"
)

// DefaultTier is the tier used when a request names none.
const DefaultTier = "default"

// RateLimitHandler serves the rate-limiting endpoints. Limiters are indexed
// by algorithm, then by tier.
type RateLimitHandler struct {
	limiters         map[string]map[string]limiter.RateLimiter
	store            limiter.Store
	metrics          *metrics.Metrics
	defaultAlgorithm string
}

// NewRateLimitHandler creates the handler.
func NewRateLimitHandler(limiters map[string]map[string]limiter.RateLimiter, store limiter.Store, m *metrics.Metrics, defaultAlgorithm string) *RateLimitHandler {
	return &RateLimitHandler{
		limiters:         limiters,
		store:            store,
		metrics:          m,
		defaultAlgorithm: defaultAlgorithm,
	}
}

// CheckRequest is the body of POST /v1/check.
type CheckRequest struct {
	Resource   string `json:"resource" binding:"required"`   // resource being accessed, e.g. "api.users.create"
	Identifier string `json:"identifier" binding:"required"` // caller identity, e.g. a user ID or API key
	Algorithm  string `json:"algorithm"`                     // optional algorithm override
	Tier       string `json:"tier"`                          // optional tier name from the configuration
	Count      int    `json:"count"`                         // permits to consume; default 1
}

// CheckResponse is returned by check and status calls.
type CheckResponse struct {
	Allowed    bool   `json:"allowed"`
	Limit      int    `json:"limit"`
	Remaining  int    `json:"remaining"`
	ResetAt    string `json:"reset_at"`
	RetryAfter *int   `json:"retry_after,omitempty"` // whole seconds, rounded up
}

// Check handles POST /v1/check.
func (h *RateLimitHandler) Check(c *gin.Context) {
	start := time.Now()

	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must not be negative"})
		return
	}
	if req.Count == 0 {
		req.Count = 1
	}

	algorithm, lim, ok := h.limiter(req.Algorithm, req.Tier)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown algorithm or tier"})
		return
	}

	key := composeKey(req.Tier, req.Identifier+":"+req.Resource)
	result, err := lim.AllowN(c.Request.Context(), key, req.Count)
	if err != nil {
		h.decisionError(c, err)
		return
	}

	keyPrefix, _, _ := strings.Cut(req.Resource, ".")
	h.metrics.RecordRequest(algorithm, keyPrefix, result.Allowed, time.Since(start).Seconds())

	writeRateLimitHeaders(c, result)
	status := http.StatusOK
	if !result.Allowed {
		status = http.StatusTooManyRequests
	}
	c.JSON(status, toResponse(result))
}

// GetStatus handles GET /v1/status/:key. The key is the same
// "identifier:resource" pair used by check; algorithm and tier come from
// query parameters. It never consumes permits.
func (h *RateLimitHandler) GetStatus(c *gin.Context) {
	_, lim, ok := h.limiter(c.Query("algorithm"), c.Query("tier"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown algorithm or tier"})
		return
	}

	key := composeKey(c.Query("tier"), c.Param("key"))
	result, err := lim.Peek(c.Request.Context(), key)
	if err != nil {
		h.decisionError(c, err)
		return
	}

	writeRateLimitHeaders(c, result)
	c.JSON(http.StatusOK, toResponse(result))
}

// Reset handles POST /v1/reset/:key. It clears state for one key under one
// algorithm and tier.
func (h *RateLimitHandler) Reset(c *gin.Context) {
	_, lim, ok := h.limiter(c.Query("algorithm"), c.Query("tier"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown algorithm or tier"})
		return
	}

	key := composeKey(c.Query("tier"), c.Param("key"))
	if err := lim.Reset(c.Request.Context(), key); err != nil {
		h.decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reset"})
}

// Health handles GET /health. It reports 503 when the store is unreachable,
// so orchestrators stop routing to an instance that cannot decide.
func (h *RateLimitHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()

	if err := h.store.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// limiter resolves an algorithm/tier pair, applying defaults for empty
// values. The returned string is the resolved algorithm name.
func (h *RateLimitHandler) limiter(algorithm, tier string) (string, limiter.RateLimiter, bool) {
	if algorithm == "" {
		algorithm = h.defaultAlgorithm
	}
	if tier == "" {
		tier = DefaultTier
	}
	tiers, ok := h.limiters[algorithm]
	if !ok {
		return algorithm, nil, false
	}
	lim, ok := tiers[tier]
	return algorithm, lim, ok
}

func (h *RateLimitHandler) decisionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, limiter.ErrExceedsLimit), errors.Is(err, limiter.ErrInvalidCount):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		log.Printf("rate limit decision failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rate limit check failed"})
	}
}

// composeKey namespaces a client-visible key by tier so that tiers with
// different limits never share counter state.
func composeKey(tier, key string) string {
	if tier == "" {
		tier = DefaultTier
	}
	return tier + ":" + key
}

func toResponse(r *limiter.Result) CheckResponse {
	resp := CheckResponse{
		Allowed:   r.Allowed,
		Limit:     r.Limit,
		Remaining: r.Remaining,
		ResetAt:   r.ResetAt.UTC().Format(time.RFC3339),
	}
	if !r.Allowed {
		secs := retryAfterSeconds(r.RetryAfter)
		resp.RetryAfter = &secs
	}
	return resp
}

func writeRateLimitHeaders(c *gin.Context, r *limiter.Result) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(r.Limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(r.Remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(r.ResetAt.Unix(), 10))
	if !r.Allowed {
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(r.RetryAfter)))
	}
}

// retryAfterSeconds rounds up to whole seconds, the granularity of the
// Retry-After header; a denied request never advertises "retry now".
func retryAfterSeconds(d time.Duration) int {
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return secs
}
