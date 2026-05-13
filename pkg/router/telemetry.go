package router

import (
	"stream-metrics-route/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

var defaultTelemetry telemetry.Telemetry

var metricNamespace string = "stream_router"

var (
	routerInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "info",
			Help:      "router info",
		}, []string{"route_name", "upstream_type", "upstream_url", "upstream_info"},
	)

	routerTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "timeseries_total",
			Help:      "Count of handle timeseries total",
		}, []string{"route_name"})
	routerFalseTimeseries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "timeseries_false_total",
			Help:      "Count of handle timeseries false total",
		}, []string{"route_name"})

	routerWriteDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "write_duration_seconds",
			Help:      "Duration of write operations to remote stores",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"route_name"})

	routerErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "errors_total",
			Help:      "Count of errors by route name and error type",
		}, []string{"route_name", "error_type"})

	routerQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "queue_depth",
			Help:      "Current queue depth for each route",
		}, []string{"route_name"})

	routerBackendHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "backend_health",
			Help:      "Health status of each backend (1=healthy, 0=unhealthy)",
		}, []string{"route_name", "backend"})

	routerCircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		}, []string{"route_name"})
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(routerTimeseries)
	defaultTelemetry.Register(routerFalseTimeseries)
	defaultTelemetry.Register(routerInfo)
	defaultTelemetry.Register(routerWriteDuration)
	defaultTelemetry.Register(routerErrors)
	defaultTelemetry.Register(routerQueueDepth)
	defaultTelemetry.Register(routerBackendHealth)
	defaultTelemetry.Register(routerCircuitBreakerState)
}
