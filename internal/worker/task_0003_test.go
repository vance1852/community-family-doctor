package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestRuntimePreservesCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := New(nil, nil, nil, slog.Default(), "test", time.Millisecond, time.Second, 1)
	if err := runtime.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("worker cancellation error = %v", err)
	}
}
