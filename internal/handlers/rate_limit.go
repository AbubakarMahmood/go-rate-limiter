package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AbubakarMahmood1/go-rate-limiter/internal/metrics"
	"github.com/AbubakarMahmood1/go-rate-limiter/pkg/limiter"
	"github.com/gin-gonic/gin"
)

// RateLimitHandler handles rate limiting HTTP requests
type RateLimitHandler struct {
	limiters         map[string]limiter.RateLimiter // algorithm name -> limiter
	metrics          *metrics.Metrics
	defaultAlgorithm string // default algorithm name
}

// NewRateLimitHandler creates a new rate limit handler
func NewRateLimitHandler(limiters map[string]limiter.RateLimiter, metrics *metrics.Metrics, defaultAlgorithm string) *RateLimitHandler {
	return &RateLimitHandler{
		limiters:         limiters,
		metrics:          metrics,
		defaultAlgorithm: defaultAlgorithm,
	}
}

// CheckRequest represents a rate limit check request
type CheckRequest struct {
	Resource   string `json:"resource" binding:"required"`   // Resource being accessed (e.g., "api.users.create")
	Identifier string `json:"identifier" binding:"required"` // User/client identifier
	Algorithm  string `json:"algorithm"`                     // Optional: override default algorithm
	Count      int    `json:"count"`                         // Optional: number of tokens to consume (default: 1)
}

// CheckResponse represents a rate limit check response
type CheckResponse struct {
	Allowed    bool   `json:"allowed"`
	Limit      int    `json:"limit"`
	Remaining  int    `json:"remaining"`
	ResetAt    string `json:"reset_at"`
	RetryAfter *int   `json:"retry_after,omitempty"` // Seconds to wait before retrying
}

// Check handles POST /v1/check - check if request is allowed
func (h *RateLimitHandler) Check(c *gin.Context) {
	start := time.Now()

	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default to 1 token if not specified
	if req.Count == 0 {
		req.Count = 1
	}

	// Select algorithm
	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = h.defaultAlgorithm
	}

	limiterInstance, ok := h.limiters[algorithm]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid algorithm"})
		return
	}

	// Create rate limit key
	key := req.Identifier + ":" + req.Resource

	// Check rate limit
	allowed, info, err := limiterInstance.AllowN(key, req.Count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rate limit check failed"})
		return
	}

	// Record metrics
	latency := time.Since(start).Seconds()
	keyPrefix := strings.Split(req.Resource, ".")[0]
	h.metrics.RecordRequest(algorithm, keyPrefix, allowed, latency)

	// Build response
	resp := CheckResponse{
		Allowed:   allowed,
		Limit:     info.Limit,
		Remaining: info.Remaining,
		ResetAt:   info.ResetAt.Format(time.RFC3339),
	}

	if info.RetryAfter != nil {
		retrySeconds := int(info.RetryAfter.Seconds())
		resp.RetryAfter = &retrySeconds
	}

	// Set standard rate limit headers
	c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", info.Limit))
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", info.Remaining))
	c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", info.ResetAt.Unix()))
	if info.RetryAfter != nil {
		c.Header("Retry-After", fmt.Sprintf("%d", int(info.RetryAfter.Seconds())))
	}

	// Return 429 if rate limited
	if !allowed {
		c.JSON(http.StatusTooManyRequests, resp)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// StatusRequest represents a status check request
type StatusRequest struct {
	Algorithm string `form:"algorithm"` // Optional: algorithm to check
}

// GetStatus handles GET /v1/status/:key - get current limit status
func (h *RateLimitHandler) GetStatus(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	var req StatusRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Select algorithm
	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = h.defaultAlgorithm
	}

	limiterInstance, ok := h.limiters[algorithm]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid algorithm"})
		return
	}

	// Check current status without consuming tokens
	allowed, info, err := limiterInstance.AllowN(key, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "status check failed"})
		return
	}

	resp := CheckResponse{
		Allowed:   allowed,
		Limit:     info.Limit,
		Remaining: info.Remaining,
		ResetAt:   info.ResetAt.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, resp)
}

// Reset handles POST /v1/reset/:key - reset limits for a key
func (h *RateLimitHandler) Reset(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	var req StatusRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Select algorithm
	algorithm := req.Algorithm
	if algorithm == "" {
		algorithm = h.defaultAlgorithm
	}

	limiterInstance, ok := h.limiters[algorithm]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid algorithm"})
		return
	}

	// Reset the limit
	if err := limiterInstance.Reset(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rate limit reset successfully"})
}

// Health handles GET /health - health check
func (h *RateLimitHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}
