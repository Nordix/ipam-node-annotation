package log

import (
	"context"
	"fmt"
	golog "log"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapLogger returns a default logger to use in NewContext.
// Allowed levels are "debug" or "trace", anything else is "info".
func ZapLogger(output, level string) (*zap.Logger, error) {
	if output == "" {
		return nil, fmt.Errorf("No output")
	}

	var lvl = 0
	switch strings.ToLower(level) {
	case "debug":
		lvl = -1
	case "trace":
		lvl = -2
	default:
		lvl, _ = strconv.Atoi(level)
		if lvl > 0 {
			lvl = -lvl
		}
	}

	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(lvl))
	zc.DisableStacktrace = true
	zc.DisableCaller = true
	zc.OutputPaths = []string{output}
	zc.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return zc.Build()
}

// NewContext returns a context with a logr.Logger based on the passed zap.Logger.
// If the passed context already contains a logr.Logger it is returned without
// modifications.
func NewContext(ctx context.Context, z *zap.Logger) context.Context {
	// Just return if the context already has a logger
	if _, err := logr.FromContext(ctx); err == nil {
		return ctx
	}
	logger := zapr.NewLogger(z)
	return logr.NewContext(ctx, logger)
}

// zapLogger returns the underlying zap.Logger.
// NOTE; If exported this breaks the use of different log implementations!
func zapLogger(logger logr.Logger) *zap.Logger {
	if underlier, ok := logger.GetSink().(zapr.Underlier); ok {
		return underlier.GetUnderlying()
	} else {
		return nil
	}
}

// Fatal emit a log message on error level and quit
func Fatal(ctx context.Context, msg string, keysAndValues ...any) {
	logger, err := logr.FromContext(ctx)
	if err == nil {
		if z := zapLogger(logger); z != nil {
			z.Sugar().Fatalw(msg, keysAndValues...)
			os.Exit(1)
		}
	}

	// Fallback to go default
	golog.Fatal(msg, keysAndValues)
}
