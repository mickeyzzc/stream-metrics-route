package receive

import (
	"context"
	"io"
	"net/http"
	"stream-metrics-route/pkg/remote"
	"stream-metrics-route/pkg/router"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

const (
	defaultMaxRequestSize = 100 * 1024 * 1024
	defaultWriteTimeout  = 30 * time.Second
)

type ReceiveRoute interface {
	BuildRoute(...*remote.RemoteWriterUrl)
	Handler() func(*gin.Context)
}

type Receive struct {
	Upstream map[int]*remote.RemoteWriterUrl
	uplen    int
	MaxSize  int64
	Timeout  time.Duration
}

var writeTasker int32 = 0

func NewReceive(maxSize int64, timeout time.Duration) *Receive {
	if maxSize <= 0 {
		maxSize = defaultMaxRequestSize
	}
	if timeout <= 0 {
		timeout = defaultWriteTimeout
	}
	return &Receive{
		MaxSize: maxSize,
		Timeout: timeout,
	}
}

func (r *Receive) BuildRoute(rwu ...*remote.RemoteWriterUrl) {
	r.Upstream = make(map[int]*remote.RemoteWriterUrl)
	for index, rw := range rwu {
		r.Upstream[index] = rw
	}
	r.uplen = len(rwu)
}

func (r *Receive) Handler() func(c *gin.Context) {
	return func(c *gin.Context) {
		start := time.Now()

		if c.Request.ContentLength > r.MaxSize {
			streamReceiveErrors.WithLabelValues("content_length_exceeded").Inc()
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"code": http.StatusRequestEntityTooLarge,
				"msg":  "request body too large",
			})
			return
		}

		limitedReader := io.LimitReader(c.Request.Body, r.MaxSize)
		compressed, err := io.ReadAll(limitedReader)
		if err != nil {
			streamReceiveErrors.WithLabelValues("read_error").Inc()
			defaultTelemetry.Logger.Error("failed to read request body", "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code": http.StatusBadRequest,
				"msg":  "failed to read request body",
			})
			return
		}

		if len(compressed) == 0 {
			streamReceiveErrors.WithLabelValues("empty_body").Inc()
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code": http.StatusBadRequest,
				"msg":  "empty request body",
			})
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			streamReceiveErrors.WithLabelValues("snappy_decode_error").Inc()
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code": http.StatusBadRequest,
				"msg":  "failed to decode snappy data",
			})
			return
		}

		streamReceiveDataByte.WithLabelValues(c.Request.RequestURI).Add(float64(len(reqBuf)))

		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			streamReceiveErrors.WithLabelValues("protobuf_decode_error").Inc()
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code": http.StatusBadRequest,
				"msg":  "failed to decode protobuf data",
			})
			return
		}

		seriesCount := int64(len(req.Timeseries))
		sampleCount := int64(0)
		for _, ts := range req.Timeseries {
			sampleCount += int64(len(ts.Samples))
		}

		streamReceiveSeriesData.WithLabelValues(c.Request.RequestURI).Add(float64(seriesCount))
		streamReceiveSamplesData.WithLabelValues(c.Request.RequestURI).Add(float64(sampleCount))

		defaultTelemetry.Logger.Debug("Receive data", "size", len(reqBuf), "len", seriesCount)

		if len(req.Timeseries) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"code": http.StatusOK,
				"msg":  "ok",
				"data": gin.H{
					"series_count": 0,
				},
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), r.Timeout)
		defer cancel()

		results := router.Store(ctx, req.Timeseries)

		duration := time.Since(start).Seconds()
		streamReceiveDuration.WithLabelValues(c.Request.RequestURI).Observe(duration)

		hasErrors := false
		errorMessages := make([]string, 0)
		for _, result := range results {
			if result.Error != nil {
				hasErrors = true
				errorMessages = append(errorMessages, result.Error.Error())
			}
		}

		if hasErrors {
			streamReceiveErrors.WithLabelValues("store_error").Inc()
			defaultTelemetry.Logger.Error("store errors", "errors", errorMessages)

			if router.GetRouters().IsHealthy() {
				c.JSON(http.StatusAccepted, gin.H{
					"code": http.StatusAccepted,
					"msg":  "partial failure",
					"data": gin.H{
						"series_count":  seriesCount,
						"sample_count":  sampleCount,
						"errors":        errorMessages,
					},
				})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"code": http.StatusServiceUnavailable,
					"msg":  "all backends unavailable",
					"data": gin.H{
						"series_count":  seriesCount,
						"sample_count":  sampleCount,
						"errors":        errorMessages,
					},
				})
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": http.StatusOK,
			"msg":  "ok",
			"data": gin.H{
				"series_count": seriesCount,
				"sample_count": sampleCount,
			},
		})
	}
}
