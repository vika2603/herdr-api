package plugin

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestShutdownContextCancelsOnASignalHerdrSends(t *testing.T) {
	if len(shutdownSignals) == 0 {
		t.Fatal("no shutdown signals are declared for this platform")
	}
	for _, sig := range shutdownSignals {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, stop := ShutdownContext(context.Background())
			defer stop()

			process, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatalf("find this process: %v", err)
			}
			if err := process.Signal(sig); err != nil {
				t.Skipf("this platform cannot deliver %v to itself: %v", sig, err)
			}
			select {
			case <-ctx.Done():
				if !errors.Is(ctx.Err(), context.Canceled) {
					t.Errorf("context ended with %v, want Canceled", ctx.Err())
				}
			case <-time.After(5 * time.Second):
				t.Errorf("%v did not cancel the context", sig)
			}
		})
	}
}

func TestShutdownContextKeepsTheParentsCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := ShutdownContext(parent)
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Error("cancelling the parent did not cancel the returned context")
	}
}

// stop has to restore the default handling, or a later signal would be
// swallowed by a handler nobody is listening to.
func TestShutdownContextStopIsIdempotent(t *testing.T) {
	ctx, stop := ShutdownContext(context.Background())
	stop()
	stop()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("stop left the context in %v, want Canceled", ctx.Err())
	}
}
