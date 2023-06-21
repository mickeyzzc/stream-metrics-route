package telemetry

import (
	"os"

	"golang.org/x/exp/slog"
)

var opts = slog.NewJSONHandler(
	os.Stdout,
	&slog.HandlerOptions{
		AddSource: true,
		Level:     LevelTrace,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			/*
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
			*/
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)

				switch {
				case level < LevelDebug:
					a.Value = slog.StringValue("TRACE")
				case level < LevelInfo:
					a.Value = slog.StringValue("DEBUG")
				case level < LevelWarning:
					a.Value = slog.StringValue("INFO")
				case level < LevelError:
					a.Value = slog.StringValue("WARNING")
				default:
					a.Value = slog.StringValue("ERROR")
				}
			}

			return a
		},
	},
)

var defaultLogger = slog.New(opts)

const (
	LevelTrace   = slog.Level(-8)
	LevelDebug   = slog.LevelDebug
	LevelInfo    = slog.LevelInfo
	LevelWarning = slog.LevelWarn
	LevelError   = slog.LevelError
)
