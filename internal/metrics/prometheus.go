package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the rate limiter
type Metrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestsAllowed *prometheus.CounterVec
	RequestsDenied  *prometheus.CounterVec
	Latency         *prometheus.HistogramVec
	RedisErrors     *prometheus.CounterVec
	StoreOperations *prometheus.HistogramVec
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_requests_total",
				Help: "Total number of rate limit check requests",
			},
			[]string{"algorithm", "key_prefix"},
		),

		RequestsAllowed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_requests_allowed",
				Help: "Number of requests allowed",
			},
			[]string{"algorithm", "key_prefix"},
		),

		RequestsDenied: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_requests_denied",
				Help: "Number of requests denied",
			},
			[]string{"algorithm", "key_prefix"},
		),

		Latency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rate_limiter_latency_seconds",
				Help:    "Request latency in seconds",
				Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1},
			},
			[]string{"algorithm", "operation"},
		),

		RedisErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_redis_errors_total",
				Help: "Total number of Redis errors",
			},
			[]string{"operation"},
		),

		StoreOperations: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rate_limiter_store_operations_seconds",
				Help:    "Store operation latency in seconds",
				Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1},
			},
			[]string{"store_type", "operation"},
		),
	}
}

// RecordRequest records a rate limit check
func (m *Metrics) RecordRequest(algorithm, keyPrefix string, allowed bool, latency float64) {
	m.RequestsTotal.WithLabelValues(algorithm, keyPrefix).Inc()

	if allowed {
		m.RequestsAllowed.WithLabelValues(algorithm, keyPrefix).Inc()
	} else {
		m.RequestsDenied.WithLabelValues(algorithm, keyPrefix).Inc()
	}

	m.Latency.WithLabelValues(algorithm, "check").Observe(latency)
}

// RecordRedisError records a Redis error
func (m *Metrics) RecordRedisError(operation string) {
	m.RedisErrors.WithLabelValues(operation).Inc()
}

// RecordStoreOperation records a store operation
func (m *Metrics) RecordStoreOperation(storeType, operation string, latency float64) {
	m.StoreOperations.WithLabelValues(storeType, operation).Observe(latency)
}
