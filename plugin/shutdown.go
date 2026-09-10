package plugin

import (
	"context"
	"os/signal"
)

// ShutdownContext returns a copy of parent that is cancelled when Herdr asks
// the process to stop, so that a long-running entrypoint can finish what it
// is doing and exit rather than being killed mid-write.
//
// A pane entrypoint needs this: it runs until the user closes the pane, and
// nothing else tells it to stop. Startup hooks, event hooks and actions are
// short-lived and normally do not.
//
//	func main() {
//		ctx, stop := plugin.ShutdownContext(context.Background())
//		defer stop()
//		os.Exit(plugin.Run(ctx, handlers))
//	}
//
// The signals are the ones herdr 0.9.0 actually sends. Closing a pane
// delivers SIGHUP and then SIGTERM to the process in it, which was measured
// against a running server rather than assumed; SIGINT is included because a
// terminal sends it when the user interrupts, not because Herdr does.
//
// Calling the returned stop function releases the signal handlers and
// restores the default behaviour, so defer it.
func ShutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, shutdownSignals...)
}
