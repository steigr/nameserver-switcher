// Package metrics provides Prometheus metrics for the nameserver-switcher.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics.
type Metrics struct {
	RequestsTotal     *prometheus.CounterVec
	RequestDuration   *prometheus.HistogramVec
	ResolverUsed      *prometheus.CounterVec
	PatternMatches    *prometheus.CounterVec
	CNAMEMatches      *prometheus.CounterVec
	Errors            *prometheus.CounterVec
	ActiveConnections prometheus.Gauge
	DNSResponseCodes  *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with all metrics registered.
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "nameserver_switcher"
	}

	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Total number of DNS requests received",
			},
			[]string{"protocol", "type"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Duration of DNS request processing",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"resolver"},
		),
		ResolverUsed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "resolver_used_total",
				Help:      "Total number of times each resolver was used",
			},
			[]string{"resolver"},
		),
		PatternMatches: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "pattern_matches_total",
				Help:      "Total number of request pattern matches",
			},
			[]string{"pattern"},
		),
		CNAMEMatches: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cname_matches_total",
				Help:      "Total number of CNAME pattern matches",
			},
			[]string{"pattern"},
		),
		Errors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total number of errors",
			},
			[]string{"type"},
		),
		ActiveConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_connections",
				Help:      "Number of active connections",
			},
		),
		DNSResponseCodes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "dns_response_codes_total",
				Help:      "Total number of DNS responses by response code",
			},
			[]string{"rcode"},
		),
	}
}

// RecordRequest records a DNS request metric.
func (m *Metrics) RecordRequest(protocol, qtype string) {
	m.RequestsTotal.WithLabelValues(protocol, qtype).Inc()
}

// RecordDuration records the duration of a request.
func (m *Metrics) RecordDuration(resolver string, duration float64) {
	m.RequestDuration.WithLabelValues(resolver).Observe(duration)
}

// RecordResolverUsed records which resolver was used.
func (m *Metrics) RecordResolverUsed(resolver string) {
	m.ResolverUsed.WithLabelValues(resolver).Inc()
}

// RecordPatternMatch records a request pattern match.
func (m *Metrics) RecordPatternMatch(pattern string) {
	m.PatternMatches.WithLabelValues(pattern).Inc()
}

// RecordCNAMEMatch records a CNAME pattern match.
func (m *Metrics) RecordCNAMEMatch(pattern string) {
	m.CNAMEMatches.WithLabelValues(pattern).Inc()
}

// RecordError records an error.
func (m *Metrics) RecordError(errType string) {
	m.Errors.WithLabelValues(errType).Inc()
}

// RecordResponseCode records a DNS response code.
func (m *Metrics) RecordResponseCode(rcode string) {
	m.DNSResponseCodes.WithLabelValues(rcode).Inc()
}

// IncActiveConnections increments active connections.
func (m *Metrics) IncActiveConnections() {
	m.ActiveConnections.Inc()
}

// DecActiveConnections decrements active connections.
func (m *Metrics) DecActiveConnections() {
	m.ActiveConnections.Dec()
}
