package router

import (
	"context"

	"github.com/prometheus/prometheus/prompb"
)

type RemoteStore interface {
	Store(ctx context.Context, req []prompb.TimeSeries) error
	IsHealthy() bool
	GetStats() map[string]interface{}
}
