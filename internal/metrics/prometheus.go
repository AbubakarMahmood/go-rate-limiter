// Package metrics exposes the service's Prometheus instrumentation.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds the Prometheus collectors for rate-limit decisions.
//
// key_prefix is the first dot-separated segment of the resource (e.g. "api"
// for "api.users.create"), which keeps label cardinality bounded by the
// caller's resource taxonomy rather than by raw identifiers.
type Metrics struct {
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
}

// New registers the collectors with reg and returns them. Tests pass their
// own registry; main passes prometheus.DefaultRegisterer.
func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		Requests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_requests_total",
				Help: "Rate-limit checks processed, by algorithm and outcome.",
			},
			[]string{"algorithm", "key_prefix", "result"},
		),
		Duration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rate_limiter_request_duration_seconds",
				Help:    "Latency of rate-limit checks.",
				Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1},
			},
			[]string{"algorithm"},
		),
	}
}

// RecordRequest records one rate-limit decision.
func (m *Metrics) RecordRequest(algorithm, keyPrefix string, allowed bool, seconds float64) {
	result := "allowed"
	if !allowed {
		result = "denied"
	}
	m.Requests.WithLabelValues(algorithm, keyPrefix, result).Inc()
	m.Duration.WithLabelValues(algorithm).Observe(seconds)
}
