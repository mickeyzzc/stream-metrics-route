package receive

import (
	"io"
	"net/http"

	"stream-metrics-route/pkg/remote"
	"stream-metrics-route/pkg/router"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type ReceiveRoute interface {
	BuildRoute(...*remote.RemoteWriterUrl)
	Handler() func(*gin.Context)
}

type Receive struct {
	Upstream map[int]*remote.RemoteWriterUrl
	uplen    int
}

// var sendSamplesChan map[int]*[]prompb.TimeSeries
var writeTasker int32 = 0

func (r *Receive) BuildRoute(rwu ...*remote.RemoteWriterUrl) {
	r.Upstream = make(map[int]*remote.RemoteWriterUrl)
	for index, rw := range rwu {
		r.Upstream[index] = rw
	}
	r.uplen = len(rwu)
}

// Handler returns a Gin handler function that receives and processes Prometheus
// samples in the received payload. It reads the payload, uncompresses it, and
// extracts the samples. Then it sorts them by hash (instance and pod labels) to
// send them to the corresponding upstream servers.
//
// It takes no parameters.
// Returns a Gin handler function.
func (r *Receive) Handler() func(c *gin.Context) {
	return func(c *gin.Context) {

		compressed, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		// metrics
		streamReceiveData.WithLabelValues(c.Request.RequestURI).Add(float64(len(reqBuf)))
		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		//

		streamReceiveSeriesData.WithLabelValues(c.Request.RequestURI).Add(float64(len(req.Timeseries)))
		defaultTelemetry.Logger.Debug("Receive data", "size", len(reqBuf), "len", len(req.Timeseries))
		if len(req.Timeseries) == 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		routers := router.GetRouters()
		go routers.Store(c.Request.Context(), req.Timeseries)
	}
}
