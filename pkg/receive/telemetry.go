package receive

import (
	"net/http"
	"stream-metrics-route/pkg/router"
	"stream-metrics-route/pkg/telemetry"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	defaultTelemetry telemetry.Telemetry
	metricNamespace  = "stream"
)

var (
	streamReceiveRemoteWriteDurationsHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_request_durations",
			Help:      "HTTP latency distributions for remote write requests.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"remote", "code"},
	)

	streamReceiveDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "receive_request_duration_seconds",
			Help:      "Duration of receiving and processing requests.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"src_service"},
	)

	streamReceiveDataByte = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_data_bytes_total",
			Help:      "Total bytes of received data.",
		}, []string{"src_service"},
	)

	streamReceiveSeriesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_series_total",
			Help:      "Total number of received time series.",
		}, []string{"src_service"},
	)

	streamReceiveSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_samples_total",
			Help:      "Total number of received samples.",
		}, []string{"src_service"},
	)

	streamReceiveRemoteWriteData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_bytes_total",
			Help:      "Total bytes sent to remote write endpoints.",
		}, []string{"remote", "code"},
	)

	streamReceiveRemoteWriteSeriesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_series_total",
			Help:      "Total number of series sent to remote write endpoints.",
		}, []string{"remote", "code"},
	)

	streamReceiveRemoteWriteSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_samples_total",
			Help:      "Total number of samples sent to remote write endpoints.",
		}, []string{"remote", "code"},
	)

	streamReceiveDropSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_drop_samples_total",
			Help:      "Total number of dropped samples.",
		}, []string{"src_service"},
	)

	streamReceiveErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_errors_total",
			Help:      "Total number of errors by type.",
		}, []string{"error_type"},
	)

	streamReceiveRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_requests_total",
			Help:      "Total number of received requests.",
		}, []string{"status_code"},
	)

	streamReceiveInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "receive_in_flight",
			Help:      "Number of requests currently being processed.",
		},
	)
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(streamReceiveRemoteWriteDurationsHistogram)
	defaultTelemetry.Register(streamReceiveDuration)
	defaultTelemetry.Register(streamReceiveDataByte)
	defaultTelemetry.Register(streamReceiveSamplesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteData)
	defaultTelemetry.Register(streamReceiveSeriesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteSeriesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteSamplesData)
	defaultTelemetry.Register(streamReceiveDropSamplesData)
	defaultTelemetry.Register(streamReceiveErrors)
	defaultTelemetry.Register(streamReceiveRequestsTotal)
	defaultTelemetry.Register(streamReceiveInFlight)
}

type response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func CheckHealthy(c *gin.Context) bool {
	routers := router.GetRouters()
	return routers.IsHealthy()
}

func CheckReady(c *gin.Context) {
	if CheckHealthy(nil) {
		c.JSON(http.StatusOK, response{Code: 2000, Msg: "ok", Data: nil})
	} else {
		c.JSON(http.StatusServiceUnavailable, response{Code: 5001, Msg: "backend unhealthy", Data: nil})
	}
}

func CheckWriteTask(checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			defaultTelemetry.Logger.Warn("check write task num.", "task_num", writeTasker)
		}
	}
}
