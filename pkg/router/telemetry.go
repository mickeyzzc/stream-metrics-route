package router

import (
	"stream-metrics-route/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

var defaultTelemetry telemetry.Telemetry

var metricNamespace string = "stream_router"

var (
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
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(routerTimeseries)
	defaultTelemetry.Register(routerFalseTimeseries)
}
