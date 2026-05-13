package remote

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type RemoteWriterUrl struct {
	Addr string
	mu   sync.RWMutex

	httpClient *http.Client

	timeout          time.Duration
	maxRetries       int
	initialBackoff   time.Duration
	maxBackoff       time.Duration

	failures  int64
	successes int64
}

const defaultBackoff = 0
const maxErrMsgLen = 1024

type RecoverableError struct {
	error
	retryAfter model.Duration
}

func NewRemoteWriterUrl(addr string) *RemoteWriterUrl {
	return &RemoteWriterUrl{
		Addr:           addr,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		timeout:        30 * time.Second,
		maxRetries:     3,
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     5 * time.Second,
	}
}

func (r *RemoteWriterUrl) Store(ctx context.Context, tsdata []prompb.TimeSeries) (int, error) {
	pBuf := proto.NewBuffer(nil)
	remoteWriteTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))

	var lastErr error
	var backoff time.Duration

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return http.StatusRequestTimeout, ctx.Err()
			case <-time.After(backoff):
			}

			backoff = time.Duration(math.Min(float64(backoff*2), float64(r.maxBackoff)))
		}

		req, err := buildWriteRequest(tsdata, nil, pBuf, nil)
		if err != nil {
			remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
			defaultTelemetry.Logger.Error("buildWriteRequest error", "err", err)
			return http.StatusInternalServerError, err
		}

		if len(req) == 0 {
			remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
			defaultTelemetry.Logger.Error("buildWriteRequest nil")
			return http.StatusInternalServerError, err
		}

		statusCode, err := r.store(ctx, req)
		lastErr = err

		if err == nil {
			r.recordSuccess()
			return statusCode, nil
		}

		r.recordFailure()

		if isNonRetryable(err) {
			remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
			return statusCode, err
		}

		if isRetryable(err) {
			continue
		}

		break
	}

	remoteWriteFalseTimeseries.WithLabelValues(r.Addr).Add(float64(len(tsdata)))
	defaultTelemetry.Logger.Error("max retries exceeded", "addr", r.Addr, "err", lastErr)
	return http.StatusInternalServerError, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (r *RemoteWriterUrl) store(c context.Context, req []byte) (int, error) {
	httpReq, err := http.NewRequest("POST", r.Addr, bytes.NewReader(req))
	if err != nil {
		return http.StatusBadRequest, err
	}

	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "stream-metrics-route")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	ctx, cancel := context.WithTimeout(c, r.timeout)
	defer cancel()

	httpResp, err := r.httpClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		defaultTelemetry.Logger.Error("http request error", "err", err, "addr", r.Addr)
		if httpResp == nil {
			return http.StatusServiceUnavailable, &RecoverableError{error: err, retryAfter: defaultBackoff}
		}
		return httpResp.StatusCode, &RecoverableError{error: err, retryAfter: defaultBackoff}
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
		return httpResp.StatusCode, &RecoverableError{error: err, retryAfter: defaultBackoff}
	}

	return httpResp.StatusCode, err
}

func (r *RemoteWriterUrl) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.successes++
	r.failures = 0
	remoteWriteSuccesses.WithLabelValues(r.Addr).Inc()
}

func (r *RemoteWriterUrl) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures++
	remoteWriteFailures.WithLabelValues(r.Addr).Inc()
}

func (r *RemoteWriterUrl) IsHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.failures < 5
}

func isRetryable(err error) bool {
	if recoverable, ok := err.(*RecoverableError); ok {
		return recoverable.retryAfter > 0 || recoverable.error != nil
	}
	return false
}

func isNonRetryable(err error) bool {
	if recoverable, ok := err.(*RecoverableError); ok {
		return recoverable.retryAfter == 0
	}
	return false
}
