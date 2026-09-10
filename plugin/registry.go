package plugin

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/vika2603/herdr-client/herdr"
)

// Plugin is a registry of handlers, one per entrypoint a manifest declares.
// It replaces the switch over action ids and the type switch over events that
// a Handlers set leaves to the caller, and it turns an id the binary does not
// serve into a dispatch error instead of a silent success.
//
//	p := plugin.New()
//	p.Startup(onStartup)
//	p.Action("show", onShow)
//	plugin.OnEvent(p, onStatusChanged)
//	os.Exit(p.Run(ctx))
//
// Registration is a program's own static description of itself, so every
// mistake in it panics rather than being reported at dispatch time: a nil
// handler, an empty id, and a second registration for an id or event already
// registered. A silent overwrite would also make CheckManifest agree with a
// manifest the binary does not actually serve.
//
// A Plugin is built once during start-up and only read afterwards, so it is
// safe for concurrent use only once registration has finished.
type Plugin struct {
	startup func(context.Context, *Env) error
	actions map[string]func(context.Context, *Env) error
	panes   map[string]func(context.Context, *Env) error
	events  map[string]eventBinder
}

// eventBinder type-asserts a decoded payload and binds the typed handler
// OnEvent registered. It reports an error rather than calling the handler
// with the wrong payload, so a mismatch counts as no handler having run.
type eventBinder func(env *Env, payload herdr.Event) (func(context.Context) error, error)

// New returns an empty registry.
func New() *Plugin {
	return &Plugin{
		actions: make(map[string]func(context.Context, *Env) error),
		panes:   make(map[string]func(context.Context, *Env) error),
		events:  make(map[string]eventBinder),
	}
}

// Startup registers the handler for the [[startup]] hook.
func (p *Plugin) Startup(h func(context.Context, *Env) error) {
	requireHandler(h == nil, "startup hook")
	if p.startup != nil {
		panic("plugin: startup hook is already registered")
	}
	p.startup = h
}

// Action registers the handler for the [[actions]] entry with the given local
// id, which is the id Herdr passes in HERDR_PLUGIN_ACTION_ID.
func (p *Plugin) Action(id string, h func(context.Context, *Env) error) {
	register(p.actions, "action", id, h)
}

// Pane registers the handler for the [[panes]] entry with the given local id,
// which is the id Herdr passes in HERDR_PLUGIN_ENTRYPOINT_ID.
//
// A pane command runs until the user closes the pane, so its handler should
// take the context from ShutdownContext.
func (p *Plugin) Pane(id string, h func(context.Context, *Env) error) {
	register(p.panes, "pane", id, h)
}

// OnEvent registers the handler for an [[events]] hook. The event name comes
// from the payload type's EventName method, so the caller never writes it:
//
//	plugin.OnEvent(p, func(ctx context.Context, env *plugin.Env, e *herdr.PaneAgentStatusChangedEvent) error {
//		…
//	})
//
// It is a free function rather than a method because it takes a type
// parameter. Registering a payload Herdr never delivers to a hook, such as
// PaneOutputMatchedEvent, is accepted here and reported by
// plugintest.CheckManifest, which knows which events Herdr hooks.
func OnEvent[E herdr.Event](p *Plugin, h func(context.Context, *Env, *E) error) {
	requireHandler(h == nil, "event hook")
	var zero E
	name := zero.EventName()
	if _, exists := p.events[name]; exists {
		panic(fmt.Sprintf("plugin: event %s is already registered", name))
	}
	p.events[name] = func(env *Env, payload herdr.Event) (func(context.Context) error, error) {
		// The assertion goes through any because a type assertion to a
		// pointer to a type parameter is not allowed directly.
		typed, ok := any(payload).(*E)
		if !ok {
			return nil, fmt.Errorf("plugin: event hook %s: payload is %T, want *%T", name, payload, zero)
		}
		return func(ctx context.Context) error { return h(ctx, env, typed) }, nil
	}
}

// Run loads the plugin environment, dispatches to the registered handler and
// returns the process exit code, with the contract Run(ctx, Handlers)
// documents: ExitOK, ExitHandlerError, and ExitRuntimeError when no handler
// ran, which now includes an id or event name with no registration.
func (p *Plugin) Run(ctx context.Context) int {
	return run(ctx, p.bind, os.LookupEnv, os.Stderr)
}

// Dispatch runs the handler for the entrypoint env describes and returns its
// error instead of an exit code, so that dispatch can be exercised against an
// environment built in memory rather than by running a plugin binary:
//
//	err := p.Dispatch(ctx, plugintest.Env(plugintest.Action("show")))
//
// The error is either the one the handler returned or the reason no handler
// ran: an id or event name with no registration, an entrypoint kind the
// environment does not identify, or an event envelope that failed to decode.
// Run reports those two cases as different exit codes; Dispatch does not
// distinguish them.
func (p *Plugin) Dispatch(ctx context.Context, env *Env) error {
	handler, err := p.bind(env, env.Kind())
	if err != nil {
		return err
	}
	return handler(ctx)
}

// Registered reports the entrypoints the registry serves, so that a manifest
// can be checked against the code; see plugintest.CheckManifest. The slices
// are sorted copies.
type Registered struct {
	// Startup reports whether a startup handler is registered.
	Startup bool
	// Actions, Panes and Events hold the registered action ids, pane ids and
	// dotted event names.
	Actions []string
	Panes   []string
	Events  []string
}

// Registered returns the entrypoints the registry serves.
func (p *Plugin) Registered() Registered {
	return Registered{
		Startup: p.startup != nil,
		Actions: slices.Sorted(maps.Keys(p.actions)),
		Panes:   slices.Sorted(maps.Keys(p.panes)),
		Events:  slices.Sorted(maps.Keys(p.events)),
	}
}

// bind implements binder over the registry.
func (p *Plugin) bind(env *Env, kind EntryKind) (func(context.Context) error, error) {
	switch kind {
	case KindStartup:
		if p.startup == nil {
			return nil, errNoHandler(kind)
		}
		return func(ctx context.Context) error { return p.startup(ctx, env) }, nil
	case KindAction:
		return bindByID(p.actions, env, kind, env.ActionID)
	case KindPane:
		return bindByID(p.panes, env, kind, env.EntrypointID)
	case KindEvent:
		bindEvent, ok := p.events[env.Event]
		if !ok {
			return nil, errNoRegistration(kind, env.Event)
		}
		envelope, err := env.eventEnvelope()
		if err != nil {
			return nil, err
		}
		if envelope.Data == nil {
			return nil, fmt.Errorf("event hook %s: envelope carries no data", env.Event)
		}
		return bindEvent(env, envelope.Data)
	default:
		return nil, errUnknownKind(kind)
	}
}

func bindByID(handlers map[string]func(context.Context, *Env) error, env *Env, kind EntryKind, id string) (func(context.Context) error, error) {
	h, ok := handlers[id]
	if !ok {
		return nil, errNoRegistration(kind, id)
	}
	return func(ctx context.Context) error { return h(ctx, env) }, nil
}

func register(handlers map[string]func(context.Context, *Env) error, what, id string, h func(context.Context, *Env) error) {
	requireHandler(h == nil, what+" "+id)
	if id == "" {
		panic("plugin: " + what + " id is empty")
	}
	if _, exists := handlers[id]; exists {
		panic(fmt.Sprintf("plugin: %s %q is already registered", what, id))
	}
	handlers[id] = h
}

func requireHandler(isNil bool, what string) {
	if isNil {
		panic("plugin: nil handler for " + what)
	}
}

func errNoRegistration(kind EntryKind, id string) error {
	return fmt.Errorf("plugin: no %s handler registered for %q", kind, id)
}
