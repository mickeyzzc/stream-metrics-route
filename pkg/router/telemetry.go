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
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(routerTimeseries)
	defaultTelemetry.Register(routerFalseTimeseries)
	defaultTelemetry.Register(routerInfo)
}
