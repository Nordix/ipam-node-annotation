package log

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
)

func testZapLogger(level string) *zap.Logger {
	z, _ := ZapLogger("stderr", level)
	return z
}

func TestLogger(t *testing.T) {

	ctx := NewContext(context.Background(), testZapLogger("trace"))
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Hello, world")
	logger.V(2).Info("You should see this")
	ctx = NewContext(ctx, testZapLogger("")) // no-op
	logger = logr.FromContextOrDiscard(ctx)
	logger.V(2).Info("You should STILL see this")

	ctx = NewContext(context.Background(), testZapLogger(""))
	logger = logr.FromContextOrDiscard(ctx)
	logger.V(2).Info("You should NOT see this")
	logger.V(1).Info("You should NOT see this")
	logger.Info("Info level log")

	ctx = NewContext(context.Background(), testZapLogger("10"))
	logger = logr.FromContextOrDiscard(ctx)
	logger.V(2).Info("You should see this (2)")
	logger.V(10).Info("You should see this (10)")
	logger.V(11).Info("You should NOT see this (11)")
}
