package plugin

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/vika2603/herdr-api"
)

// Exit codes Run returns. Herdr stores the exit status of every plugin command
// in its command log, so the two failure modes are kept apart: a handler that
// reported an error against an environment this package understood, and an
// environment it could not dispatch at all.
const (
	// ExitOK reports that the handler returned nil.
	ExitOK = 0
	// ExitHandlerError reports that the handler returned an error.
	ExitHandlerError = 1
	// ExitRuntimeError reports that no handler ran: the process environment is
	// not a Herdr plugin environment, its entrypoint kind is unknown, Handlers
	// has no handler for that kind, or the event envelope failed to decode.
	ExitRuntimeError = 2
)

// Handlers holds one function per entrypoint kind a manifest can declare. A
// nil field means the plugin does not serve that kind; being invoked for it is
// a configuration error, not a no-op.
type Handlers struct {
	// Startup serves a [[startup]] hook.
	Startup func(context.Context, *Env) error
	// Action serves an [[actions]] command and receives the local action id.
	Action func(context.Context, *Env, string) error
	// Event serves an [[events]] hook and receives the decoded envelope.
	Event func(context.Context, *Env, *herdr.EventEnvelope) error
	// Pane serves a [[panes]] command and receives the local pane id.
	Pane func(context.Context, *Env, string) error
}

// Run loads the plugin environment, dispatches to the handler for its
// entrypoint kind and returns the process exit code, so a plugin's main is
// os.Exit(plugin.Run(ctx, handlers)).
//
// Errors are written to stderr, which Herdr captures in its plugin command log
// alongside the exit code. See ExitOK, ExitHandlerError and ExitRuntimeError.
func Run(ctx context.Context, h Handlers) int {
	return run(ctx, h, os.LookupEnv, os.Stderr)
}

// run is Run with the environment lookup and the error output injected.
func run(ctx context.Context, h Handlers, lookup func(string) (string, bool), stderr io.Writer) int {
	env, err := LoadFrom(lookup)
	if err != nil {
		return fail(stderr, ExitRuntimeError, err)
	}
	kind := env.Kind()
	handler, err := bind(h, env, kind)
	if err != nil {
		return fail(stderr, ExitRuntimeError, err)
	}
	if err := handler(ctx); err != nil {
		return fail(stderr, ExitHandlerError, fmt.Errorf("%s: %s: %w", env.PluginID, kind, err))
	}
	return ExitOK
}

// bind selects the handler for kind and binds the arguments it takes from env.
func bind(h Handlers, env *Env, kind EntryKind) (func(context.Context) error, error) {
	switch kind {
	case KindStartup:
		if h.Startup == nil {
			return nil, errNoHandler(kind)
		}
		return func(ctx context.Context) error { return h.Startup(ctx, env) }, nil
	case KindAction:
		if h.Action == nil {
			return nil, errNoHandler(kind)
		}
		return func(ctx context.Context) error { return h.Action(ctx, env, env.ActionID) }, nil
	case KindEvent:
		if h.Event == nil {
			return nil, errNoHandler(kind)
		}
		envelope, err := env.EventEnvelope()
		if err != nil {
			return nil, fmt.Errorf("plugin: event hook %s: %w", env.Event, err)
		}
		return func(ctx context.Context) error { return h.Event(ctx, env, envelope) }, nil
	case KindPane:
		if h.Pane == nil {
			return nil, errNoHandler(kind)
		}
		return func(ctx context.Context) error { return h.Pane(ctx, env, env.EntrypointID) }, nil
	default:
		return nil, fmt.Errorf("plugin: %s entrypoint kind: neither %s, %s nor %s is set",
			kind, envEvent, envActionID, envEntrypointID)
	}
}

func errNoHandler(kind EntryKind) error {
	return fmt.Errorf("plugin: no %s handler registered", kind)
}

func fail(stderr io.Writer, code int, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	return code
}
