package kafkaclient

import (
	"os"
	"stream-metrics-route/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

var defaultTelemetry telemetry.Telemetry

var metricNamespace string = "stream_kafka"

var (
	promBatches = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "incoming_prometheus_batches_total",
			Help:      "Count of incoming prometheus batches (to be broken into individual metrics)",
		}, []string{"route_name"})
	serializeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "serialized_total",
			Help:      "Count of all serialization requests",
		}, []string{"route_name"})
	serializeFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "serialized_failed_total",
			Help:      "Count of all serialization failures",
		}, []string{"route_name"})
	objectsFiltered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "objects_filtered_total",
			Help:      "Count of all filter attempts",
		}, []string{"route_name"})
	objectsWritten = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "objects_written_total",
			Help:      "Count of all objects written to Kafka",
		}, []string{"route_name"})
	objectsFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "objects_failed_total",
			Help:      "Count of all objects write failures to Kafka",
		}, []string{"route_name"})
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(promBatches)
	defaultTelemetry.Register(serializeTotal)
	defaultTelemetry.Register(serializeFailed)
	defaultTelemetry.Register(objectsFiltered)
	defaultTelemetry.Register(objectsFailed)
	defaultTelemetry.Register(objectsWritten)
	var err error
	serializer, err = parseSerializationFormat(os.Getenv("SERIALIZATION_FORMAT"))
	if err != nil {
		defaultTelemetry.Logger.Error("couldn't create a metrics serializer")
	}
}
