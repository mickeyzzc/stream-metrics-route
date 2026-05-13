package remote

import (
	"stream-metrics-route/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

var defaultTelemetry telemetry.Telemetry

var metricNamespace string = "stream_remote_write"

var (
	remoteWriteClusterTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "cluster_timeseries_total",
			Help:      "Count of handle timeseries total",
		}, []string{"route_name"})
	remoteWriteTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "timeseries_total",
			Help:      "Count of handle timeseries total",
		}, []string{"url"})
	remoteWriteFalseTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "timeseries_false_total",
			Help:      "Count of handle timeseries false total",
		}, []string{"url"})
	remoteWriteClusterFalseTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "cluster_timeseries_false_total",
			Help:      "Count of handle timeseries false total",
		}, []string{"route_name"})

	remoteWriteSuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "successes_total",
			Help:      "Count of successful writes",
		}, []string{"url"})

	remoteWriteFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "failures_total",
			Help:      "Count of failed writes",
		}, []string{"url"})

	remoteWriteLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "latency_seconds",
			Help:      "Latency of remote write operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"url"})

	remoteWriteRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "retries_total",
			Help:      "Count of retries",
		}, []string{"url"})

	remoteWriteCircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		}, []string{"route_name"})
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(remoteWriteClusterTimeseries)
	defaultTelemetry.Register(remoteWriteClusterFalseTimeseries)
	defaultTelemetry.Register(remoteWriteTimeseries)
	defaultTelemetry.Register(remoteWriteFalseTimeseries)
	defaultTelemetry.Register(remoteWriteSuccesses)
	defaultTelemetry.Register(remoteWriteFailures)
	defaultTelemetry.Register(remoteWriteLatency)
	defaultTelemetry.Register(remoteWriteRetries)
	defaultTelemetry.Register(remoteWriteCircuitBreakerState)
}
