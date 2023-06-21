package router

import (
	"context"

	"github.com/prometheus/prometheus/prompb"
)

type RemoteStore interface {
	Store(ctx context.Context, req []prompb.TimeSeries) (int, error)
}
