// Package plugin reads the environment Herdr injects into plugin processes
// (HERDR_PLUGIN_* and the shared HERDR_* variables) and connects a plugin to
// the Herdr socket API. See docs/design.md.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/vika2603/herdr-client/herdr"
)

// Environment variables Herdr sets for the commands a plugin declares. See
// herdr v0.9.0, src/app/api/plugins/runtime.rs and
// src/app/api/plugins/panes.rs.
const (
	envHerdrEnv      = "HERDR_ENV"
	envPluginID      = "HERDR_PLUGIN_ID"
	envPluginRoot    = "HERDR_PLUGIN_ROOT"
	envConfigDir     = "HERDR_PLUGIN_CONFIG_DIR"
	envStateDir      = "HERDR_PLUGIN_STATE_DIR"
	envSocketPath    = "HERDR_SOCKET_PATH"
	envBinPath       = "HERDR_BIN_PATH"
	envWorkspaceID   = "HERDR_WORKSPACE_ID"
	envTabID         = "HERDR_TAB_ID"
	envPaneID        = "HERDR_PANE_ID"
	envActionID      = "HERDR_PLUGIN_ACTION_ID"
	envEntrypointID  = "HERDR_PLUGIN_ENTRYPOINT_ID"
	envEvent         = "HERDR_PLUGIN_EVENT"
	envContextJSON   = "HERDR_PLUGIN_CONTEXT_JSON"
	envEventJSON     = "HERDR_PLUGIN_EVENT_JSON"
	envClickedURL    = "HERDR_PLUGIN_CLICKED_URL"
	envLinkHandlerID = "HERDR_PLUGIN_LINK_HANDLER_ID"
)

// startupEvent is the value of HERDR_PLUGIN_EVENT for startup hooks. Event
// hooks carry a dotted event name there instead.
const startupEvent = "startup"

// ErrNotPluginProcess is returned by Load when HERDR_ENV is not "1", that is,
// when the process was not started by Herdr.
var ErrNotPluginProcess = errors.New("plugin: HERDR_ENV is not \"1\"")

// ErrMissingPluginID is returned by Load when HERDR_PLUGIN_ID is missing.
var ErrMissingPluginID = errors.New("plugin: HERDR_PLUGIN_ID is not set")

// ErrNoContext is returned by Context when HERDR_PLUGIN_CONTEXT_JSON was not
// set.
var ErrNoContext = errors.New("plugin: HERDR_PLUGIN_CONTEXT_JSON is not set")

// ErrNoEventEnvelope is returned by EventEnvelope when
// HERDR_PLUGIN_EVENT_JSON was not set.
var ErrNoEventEnvelope = errors.New("plugin: HERDR_PLUGIN_EVENT_JSON is not set")

// Env is the runtime environment of a plugin command.
//
// Which fields are populated depends on the entrypoint Herdr invoked; see
// Kind. Fields that Herdr did not set are empty.
type Env struct {
	// PluginID is the manifest id of the plugin being run.
	PluginID string
	// PluginRoot is the installed or linked plugin directory, which is also
	// the working directory of the command.
	PluginRoot string
	// ConfigDir holds user-editable plugin configuration.
	ConfigDir string
	// StateDir holds plugin-owned runtime state.
	StateDir string
	// SocketPath is the Herdr API socket path, a named pipe name on Windows.
	SocketPath string
	// BinPath is the running Herdr binary, the portable way to call back into
	// Herdr.
	BinPath string
	// WorkspaceID, TabID and PaneID are the ids of the invocation context when
	// Herdr had them; a popup pane process receives no PaneID.
	WorkspaceID string
	TabID       string
	PaneID      string
	// ActionID is the local action id for action commands.
	ActionID string
	// EntrypointID is the local pane id for pane commands.
	EntrypointID string
	// Event is "startup" for startup hooks and the dotted event name for
	// event hooks.
	Event string
	// ContextJSON is the raw PluginInvocationContext JSON.
	ContextJSON []byte
	// EventJSON is the raw EventEnvelope JSON of an event hook.
	EventJSON []byte
	// ClickedURL and LinkHandlerID are set when an action was invoked through
	// a link handler.
	ClickedURL    string
	LinkHandlerID string
}

// EntryKind is the kind of manifest entrypoint that started the process.
type EntryKind string

const (
	// KindUnknown means the environment carries no marker for any entrypoint
	// kind this package knows. It happens when a plugin command is run by hand
	// outside Herdr and when a future Herdr version adds an entrypoint kind.
	KindUnknown EntryKind = "unknown"
	// KindStartup is a [[startup]] hook.
	KindStartup EntryKind = "startup"
	// KindAction is an [[actions]] command.
	KindAction EntryKind = "action"
	// KindEvent is an [[events]] hook.
	KindEvent EntryKind = "event"
	// KindPane is a [[panes]] command.
	KindPane EntryKind = "pane"
)

// Load reads the plugin environment from the process environment.
func Load() (*Env, error) { return LoadFrom(os.LookupEnv) }

// LoadFrom reads the plugin environment through lookup, which has the
// semantics of os.LookupEnv and defaults to it when nil. It fails when
// HERDR_ENV is not "1" or HERDR_PLUGIN_ID is missing, and reports every other
// variable as it finds it.
func LoadFrom(lookup func(string) (string, bool)) (*Env, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	get := func(key string) string {
		value, _ := lookup(key)
		return value
	}
	if get(envHerdrEnv) != "1" {
		return nil, ErrNotPluginProcess
	}
	pluginID := get(envPluginID)
	if pluginID == "" {
		return nil, ErrMissingPluginID
	}
	env := &Env{
		PluginID:      pluginID,
		PluginRoot:    get(envPluginRoot),
		ConfigDir:     get(envConfigDir),
		StateDir:      get(envStateDir),
		SocketPath:    get(envSocketPath),
		BinPath:       get(envBinPath),
		WorkspaceID:   get(envWorkspaceID),
		TabID:         get(envTabID),
		PaneID:        get(envPaneID),
		ActionID:      get(envActionID),
		EntrypointID:  get(envEntrypointID),
		Event:         get(envEvent),
		ClickedURL:    get(envClickedURL),
		LinkHandlerID: get(envLinkHandlerID),
	}
	if raw := get(envContextJSON); raw != "" {
		env.ContextJSON = []byte(raw)
	}
	if raw := get(envEventJSON); raw != "" {
		env.EventJSON = []byte(raw)
	}
	return env, nil
}

// Kind reports which manifest entrypoint started the process. Startup hooks
// are recognised by Event, action commands by ActionID, event hooks by a
// dotted name in Event, and pane commands by EntrypointID. An environment
// without any of those markers is KindUnknown.
func (e *Env) Kind() EntryKind {
	switch {
	case e.Event == startupEvent:
		return KindStartup
	case e.ActionID != "":
		return KindAction
	case strings.Contains(e.Event, "."):
		return KindEvent
	case e.EntrypointID != "":
		return KindPane
	default:
		return KindUnknown
	}
}

// Client returns a client for the socket Herdr injected. It performs no I/O,
// so a missing SocketPath surfaces on the first call.
func (e *Env) Client(opts ...herdr.Option) *herdr.Client {
	return herdr.New(e.SocketPath, opts...)
}

// Context decodes ContextJSON.
//
// It returns ErrNoContext when Herdr set no HERDR_PLUGIN_CONTEXT_JSON, which
// happens for entrypoints invoked without an invocation context, and a decode
// error when the value is not a PluginInvocationContext. Every field of the
// result is optional, so an entrypoint reached outside a workspace decodes
// into a context whose fields are all nil.
func (e *Env) Context() (*herdr.PluginInvocationContext, error) {
	if len(e.ContextJSON) == 0 {
		return nil, ErrNoContext
	}
	var invocation herdr.PluginInvocationContext
	if err := json.Unmarshal(e.ContextJSON, &invocation); err != nil {
		return nil, fmt.Errorf("plugin: decode %s: %w", envContextJSON, err)
	}
	return &invocation, nil
}

// EventEnvelope decodes EventJSON.
//
// Only event hooks receive HERDR_PLUGIN_EVENT_JSON; every other entrypoint
// kind gets ErrNoEventEnvelope. An event name this module does not know
// surfaces as *herdr.UnknownEventError, which carries the raw payload.
func (e *Env) EventEnvelope() (*herdr.EventEnvelope, error) {
	if len(e.EventJSON) == 0 {
		return nil, ErrNoEventEnvelope
	}
	var envelope herdr.EventEnvelope
	if err := json.Unmarshal(e.EventJSON, &envelope); err != nil {
		return nil, fmt.Errorf("plugin: decode %s: %w", envEventJSON, err)
	}
	return &envelope, nil
}
