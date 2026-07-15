package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/internal/algorithms"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/handlers"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/metrics"
	"github.com/AbubakarMahmood/go-rate-limiter/internal/store"
	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRouter builds the API exactly as main does, on a memory store with a
// small default limit (5/min, burst 5) and a "premium" tier (50/min).
func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	s := store.NewMemoryStore()
	t.Cleanup(func() { _ = s.Close() })

	defaultCfg := limiter.Config{Limit: 5, Window: time.Minute, Burst: 5}
	premiumCfg := limiter.Config{Limit: 50, Window: time.Minute, Burst: 50}

	limiters := map[string]map[string]limiter.RateLimiter{
		"token_bucket": {
			"default": algorithms.NewTokenBucket(s, defaultCfg),
			"premium": algorithms.NewTokenBucket(s, premiumCfg),
		},
		"sliding_window": {
			"default": algorithms.NewSlidingWindowCounter(s, defaultCfg),
		},
		"fixed_window": {
			"default": algorithms.NewFixedWindowCounter(s, defaultCfg),
		},
	}

	h := handlers.NewRateLimitHandler(limiters, s, metrics.New(prometheus.NewRegistry()), "token_bucket")

	router := gin.New()
	v1 := router.Group("/v1")
	v1.POST("/check", h.Check)
	v1.GET("/status/:key", h.GetStatus)
	v1.POST("/reset/:key", h.Reset)
	router.GET("/health", h.Health)
	return router
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func checkBody(resource, identifier string, extra string) string {
	b := `{"resource":"` + resource + `","identifier":"` + identifier + `"`
	if extra != "" {
		b += "," + extra
	}
	return b + "}"
}

func TestCheck_AllowsAndSetsHeaders(t *testing.T) {
	router := newRouter(t)

	w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.users.create", "u1", ""))
	require.Equal(t, http.StatusOK, w.Code)

	var resp handlers.CheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Allowed)
	assert.Equal(t, 5, resp.Limit)
	assert.Equal(t, 4, resp.Remaining)
	assert.Nil(t, resp.RetryAfter)

	assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "4", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

func TestCheck_DeniesWith429(t *testing.T) {
	router := newRouter(t)

	for i := 0; i < 5; i++ {
		w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
		require.Equal(t, http.StatusOK, w.Code, "request %d", i+1)
	}

	w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	require.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp handlers.CheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Allowed)
	assert.Equal(t, 0, resp.Remaining)
	require.NotNil(t, resp.RetryAfter)
	assert.GreaterOrEqual(t, *resp.RetryAfter, 1)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestCheck_CountConsumesPermits(t *testing.T) {
	router := newRouter(t)

	w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", `"count":5,"algorithm":"fixed_window"`))
	require.Equal(t, http.StatusOK, w.Code)

	// The window is exhausted: a single further request must be denied.
	w = doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", `"algorithm":"fixed_window"`))
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestCheck_BadRequests(t *testing.T) {
	router := newRouter(t)

	cases := map[string]string{
		"missing fields":    `{"resource":"api.test"}`,
		"negative count":    checkBody("api.test", "u1", `"count":-2`),
		"unknown algorithm": checkBody("api.test", "u1", `"algorithm":"leaky_bucket"`),
		"unknown tier":      checkBody("api.test", "u1", `"tier":"platinum"`),
		"count over limit":  checkBody("api.test", "u1", `"count":6`),
		"malformed json":    `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			w := doJSON(t, router, http.MethodPost, "/v1/check", body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestCheck_TiersAreIndependent(t *testing.T) {
	router := newRouter(t)

	// Exhaust the default tier for u1.
	for i := 0; i < 5; i++ {
		doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	}
	w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	require.Equal(t, http.StatusTooManyRequests, w.Code)

	// The same identifier under the premium tier has its own budget.
	w = doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", `"tier":"premium"`))
	require.Equal(t, http.StatusOK, w.Code)

	var resp handlers.CheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 50, resp.Limit)
	assert.Equal(t, 49, resp.Remaining)
}

func TestStatus_DoesNotConsume(t *testing.T) {
	router := newRouter(t)

	doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/status/u1:api.test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp handlers.CheckResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 4, resp.Remaining, "status call %d must not consume permits", i+1)
	}
}

func TestReset_RestoresBudget(t *testing.T) {
	router := newRouter(t)

	for i := 0; i < 5; i++ {
		doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	}
	w := doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	require.Equal(t, http.StatusTooManyRequests, w.Code)

	req := httptest.NewRequest(http.MethodPost, "/v1/reset/u1:api.test", nil)
	rw := httptest.NewRecorder()
	router.ServeHTTP(rw, req)
	require.Equal(t, http.StatusOK, rw.Code)

	w = doJSON(t, router, http.MethodPost, "/v1/check", checkBody("api.test", "u1", ""))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealth_OK(t *testing.T) {
	router := newRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok"`)
}
