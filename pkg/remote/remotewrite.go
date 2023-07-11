package remote

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type RemoteWriterUrl struct {
	Addr string
	//Header http.Handler
	Client *http.Client
	//Body    []byte
	timeout time.Duration
}

const defaultBackoff = 0
const maxErrMsgLen = 1024

type RecoverableError struct {
	error
	retryAfter model.Duration
}

func NewRemoteWriterUrl(addr string) *RemoteWriterUrl {
	httpClient, err := config.NewClientFromConfig(config.DefaultHTTPClientConfig, addr)
	if err != nil {
		defaultTelemetry.Logger.Error("http client err", err)
		return nil
	}

	rt, err := config.NewRoundTripperFromConfig(config.DefaultHTTPClientConfig, addr)
	if err != nil {
		defaultTelemetry.Logger.Error("http client err", err)
		return nil
	}
	httpClient.Transport = rt
	return &RemoteWriterUrl{
		Addr:    addr,
		Client:  httpClient,
		timeout: 5 * time.Second,
	}
}

func (r *RemoteWriterUrl) Store(ctx context.Context, tsdata []prompb.TimeSeries) (int, error) {
	pBuf := proto.NewBuffer(nil)
	remoteWriteTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
	req, err := buildWriteRequest(tsdata, nil, pBuf, nil)
	if err != nil {
		remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
		defaultTelemetry.Logger.Error("buildWriteRequest error", "err", err)
		return 404, err
	}
	if len(req) == 0 {
		remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
		defaultTelemetry.Logger.Error("buildWriteRequest nil")
		return 404, err
	}
	return r.store(ctx, req)

}

// Store sends a POST request to a remote server given a context and a request.
//
// ctx: context.Context to use.
// req: []byte containing the request payload.
// int: HTTP status code of the response.
// error: any errors encountered during the request.
func (r *RemoteWriterUrl) store(c context.Context, req []byte) (int, error) {
	httpReq, err := http.NewRequest("POST", r.Addr, bytes.NewReader(req))
	if err != nil {
		return 404, err
	}

	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "stream-metrics-route")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	httpResp, err := r.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		defaultTelemetry.Logger.Error("http request error", "err", err, "addr", r.Addr)
		if nil == httpResp {
			return 500, RecoverableError{err, defaultBackoff}
		}
		return httpResp.StatusCode, RecoverableError{err, defaultBackoff}
	}

	defer func() {
		io.Copy(io.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	if httpResp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = fmt.Errorf("server returned HTTP status %s: %s", httpResp.Status, line)
	}
	if httpResp.StatusCode/100 == 5 {
		return httpResp.StatusCode, RecoverableError{err, defaultBackoff}
	}

	return httpResp.StatusCode, err
}
