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
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(remoteWriteClusterTimeseries)
	defaultTelemetry.Register(remoteWriteClusterFalseTimeseries)
	defaultTelemetry.Register(remoteWriteTimeseries)
	defaultTelemetry.Register(remoteWriteFalseTimeseries)
}
