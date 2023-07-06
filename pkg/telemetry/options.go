package telemetry

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slog"
)

type Telemetry options

type options struct {
	Logger  *slog.Logger
	Tracer  *tracer
	Metrics *prometheus.Registry
}

var defaultOptions = Telemetry{
	Logger:  defaultLogger,
	Metrics: defaultRegistry,
	Tracer:  defaultTracer,
}

func NewTelemetry() Telemetry {
	return defaultOptions
}

func (o *Telemetry) LevelSet(ctx context.Context, lvl string) {
	switch lvl {
	case "debug":
		o.Logger.Enabled(ctx, LevelDebug)
	case "warn":
		o.Logger.Enabled(ctx, LevelWarning)
	case "error":
		o.Logger.Enabled(ctx, LevelError)
	case "trace":
		o.Logger.Enabled(ctx, LevelTrace)
	default:
		o.Logger.Enabled(ctx, LevelInfo)
	}
}
