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
	return &RemoteWriterUrl{
		Addr:    addr,
		Client:  &http.Client{},
		timeout: 15 * time.Second,
	}
}

func (r *RemoteWriterUrl) Store(ctx context.Context, tsdata []prompb.TimeSeries) (int, error) {
	pBuf := proto.NewBuffer(nil)
	req, err := buildWriteRequest(tsdata, nil, pBuf, nil)
	if err != nil {
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
func (r *RemoteWriterUrl) store(ctx context.Context, req []byte) (int, error) {
	httpReq, err := http.NewRequest("POST", r.Addr, bytes.NewReader(req))
	if err != nil {
		return 404, err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "stream-metrics-route")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	httpResp, err := r.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
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
