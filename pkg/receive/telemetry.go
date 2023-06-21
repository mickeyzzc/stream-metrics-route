package receive

import (
	"net/http"
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
	// 远程写数据的耗时
	streamReceiveRemoteWriteDurationsHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                    metricNamespace,
			Name:                         "receive_remote_write_request_durations",
			Help:                         "HTTP latency distributions.",
			Buckets:                      prometheus.DefBuckets,
			NativeHistogramZeroThreshold: 0.05,
			NativeHistogramBucketFactor:  1.5,
		}, []string{"remote", "code"},
	)
	// 接收数据的大小
	streamReceiveData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_data_byte_totol",
			Help:      "",
		}, []string{"src_service"},
	)
	// 接收series
	streamReceiveSeriesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_series_totol",
			Help:      "",
		}, []string{"src_service"},
	)
	// 接收samles
	streamReceiveSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_samples_totol",
			Help:      "",
		}, []string{"src_service"},
	)
	// 发送数据的大小
	streamReceiveRemoteWriteData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_byte_totol",
			Help:      "",
		}, []string{"remote", "code"},
	)
	// 发送series
	streamReceiveRemoteWriteSeriesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "sreceive_remote_write_series_totol",
			Help:      "",
		}, []string{"remote", "code"},
	)
	// 发送samples
	streamReceiveRemoteWriteSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_remote_write_samples_totol",
			Help:      "",
		}, []string{"remote", "code"},
	)
	// drop samples
	streamReceiveDropSamplesData = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "receive_drop_samples_totol",
			Help:      "",
		}, []string{"src_service"},
	)
)

func init() {
	defaultTelemetry = telemetry.NewTelemetry()
	defaultTelemetry.Register(streamReceiveRemoteWriteDurationsHistogram)
	defaultTelemetry.Register(streamReceiveData)
	defaultTelemetry.Register(streamReceiveSamplesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteData)
	defaultTelemetry.Register(streamReceiveSeriesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteSeriesData)
	defaultTelemetry.Register(streamReceiveRemoteWriteSamplesData)
	defaultTelemetry.Register(streamReceiveDropSamplesData)
}

type response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func CheckHealthy(c *gin.Context) bool {
	// TODO
	return true
}

func CheckReady(c *gin.Context) {
	data := response{Code: 2000, Msg: "ok", Data: nil}
	c.JSON(http.StatusOK, data)
}

func CheckWriteTask(checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	for {
		select {
		case <-ticker.C:
			defaultTelemetry.Logger.Warn("check write task num.", "task_num", writeTasker)
			if writeTasker < 1 {
				return
			}
		}
	}
}
