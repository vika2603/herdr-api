package plugin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/herdr"
)

// calls records which handler of a Handlers set ran and with which argument.
type calls struct {
	kind     EntryKind
	id       string
	envelope *herdr.EventEnvelope
	env      *Env
}

// handlersRecording returns a full Handlers set writing into got, with every
// handler returning err.
func handlersRecording(got *calls, err error) Handlers {
	return Handlers{
		Startup: func(_ context.Context, env *Env) error {
			*got = calls{kind: KindStartup, env: env}
			return err
		},
		Action: func(_ context.Context, env *Env, id string) error {
			*got = calls{kind: KindAction, id: id, env: env}
			return err
		},
		Event: func(_ context.Context, env *Env, envelope *herdr.EventEnvelope) error {
			*got = calls{kind: KindEvent, envelope: envelope, env: env}
			return err
		},
		Pane: func(_ context.Context, env *Env, id string) error {
			*got = calls{kind: KindPane, id: id, env: env}
			return err
		},
	}
}

func baseVars() map[string]string {
	return map[string]string{envHerdrEnv: "1", envPluginID: "example.agent-status"}
}

func varsWith(extra map[string]string) map[string]string {
	vars := baseVars()
	for key, value := range extra {
		vars[key] = value
	}
	return vars
}

const statusEventJSON = `{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","pane_id":"pane-1","workspace_id":"ws-1","agent_status":"done"}}`

func TestRunDispatchesByKind(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]string
		wantKind EntryKind
		wantID   string
	}{
		{
			name:     "startup",
			vars:     varsWith(map[string]string{envEvent: "startup"}),
			wantKind: KindStartup,
		},
		{
			name:     "action",
			vars:     varsWith(map[string]string{envActionID: "show", envContextJSON: `{"workspace_id":"ws-1"}`}),
			wantKind: KindAction,
			wantID:   "show",
		},
		{
			name:     "event",
			vars:     varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON}),
			wantKind: KindEvent,
		},
		{
			name:     "pane",
			vars:     varsWith(map[string]string{envEntrypointID: "board"}),
			wantKind: KindPane,
			wantID:   "board",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got calls
			var stderr bytes.Buffer

			code := run(context.Background(), handlersRecording(&got, nil).bind, lookupFrom(tt.vars), &stderr)

			if code != ExitOK {
				t.Fatalf("run() = %d, want %d (stderr %q)", code, ExitOK, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if got.kind != tt.wantKind {
				t.Fatalf("handler = %q, want %q", got.kind, tt.wantKind)
			}
			if got.id != tt.wantID {
				t.Errorf("id = %q, want %q", got.id, tt.wantID)
			}
			if got.env == nil || got.env.PluginID != "example.agent-status" {
				t.Errorf("env = %+v", got.env)
			}
		})
	}
}

func TestRunPassesDecodedEnvelopeToEventHandler(t *testing.T) {
	var got calls
	vars := varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON})

	code := run(context.Background(), handlersRecording(&got, nil).bind, lookupFrom(vars), &bytes.Buffer{})

	if code != ExitOK {
		t.Fatalf("run() = %d, want %d", code, ExitOK)
	}
	if got.envelope == nil {
		t.Fatal("envelope = nil")
	}
	if got.envelope.Event != herdr.EventKindPaneAgentStatusChanged {
		t.Errorf("Event = %q, want %q", got.envelope.Event, herdr.EventKindPaneAgentStatusChanged)
	}
	data, ok := got.envelope.Data.(*herdr.PaneAgentStatusChangedEvent)
	if !ok {
		t.Fatalf("Data = %#v, want *herdr.PaneAgentStatusChangedEvent", got.envelope.Data)
	}
	if data.PaneID != "pane-1" || data.AgentStatus != herdr.AgentStatusDone {
		t.Errorf("Data = %+v", data)
	}
}

func TestRunPassesContextToHandler(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "value")
	var seen context.Context
	handlers := Handlers{
		Startup: func(ctx context.Context, _ *Env) error {
			seen = ctx
			return nil
		},
	}

	code := run(ctx, handlers.bind, lookupFrom(varsWith(map[string]string{envEvent: "startup"})), &bytes.Buffer{})

	if code != ExitOK {
		t.Fatalf("run() = %d, want %d", code, ExitOK)
	}
	if seen == nil || seen.Value(key{}) != "value" {
		t.Errorf("handler context = %v, want the one passed to run", seen)
	}
}

func TestRunHandlerError(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
	}{
		{name: "startup", vars: varsWith(map[string]string{envEvent: "startup"})},
		{name: "action", vars: varsWith(map[string]string{envActionID: "show"})},
		{name: "event", vars: varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON})},
		{name: "pane", vars: varsWith(map[string]string{envEntrypointID: "board"})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got calls
			var stderr bytes.Buffer
			handlerErr := errors.New("state directory is not writable")

			code := run(context.Background(), handlersRecording(&got, handlerErr).bind, lookupFrom(tt.vars), &stderr)

			if code != ExitHandlerError {
				t.Fatalf("run() = %d, want %d", code, ExitHandlerError)
			}
			if got.kind == "" {
				t.Fatal("no handler ran")
			}
			message := stderr.String()
			if !strings.Contains(message, handlerErr.Error()) {
				t.Errorf("stderr = %q, want it to contain %q", message, handlerErr.Error())
			}
			if !strings.Contains(message, "example.agent-status") {
				t.Errorf("stderr = %q, want it to name the plugin", message)
			}
			if !strings.HasSuffix(message, "\n") {
				t.Errorf("stderr = %q, want a trailing newline", message)
			}
		})
	}
}

func TestRunRuntimeErrors(t *testing.T) {
	// noHandlers stands for a plugin that registered nothing at all.
	noHandlers := func(*calls) Handlers { return Handlers{} }
	// allHandlers stands for a plugin that registered every kind, so a
	// recorded call proves Run dispatched where it should not have.
	allHandlers := func(got *calls) Handlers { return handlersRecording(got, nil) }

	tests := []struct {
		name       string
		vars       map[string]string
		handlers   func(*calls) Handlers
		wantStderr string
	}{
		{
			name:       "not a plugin process",
			vars:       map[string]string{envPluginID: "example.agent-status"},
			handlers:   allHandlers,
			wantStderr: "HERDR_ENV",
		},
		{
			name:       "plugin id missing",
			vars:       map[string]string{envHerdrEnv: "1"},
			handlers:   allHandlers,
			wantStderr: "HERDR_PLUGIN_ID",
		},
		{
			name:       "unknown kind",
			vars:       baseVars(),
			handlers:   allHandlers,
			wantStderr: "unknown entrypoint kind",
		},
		{
			name: "no startup handler",
			vars: varsWith(map[string]string{envEvent: "startup"}),
			handlers: func(got *calls) Handlers {
				return Handlers{Action: handlersRecording(got, nil).Action}
			},
			wantStderr: "no startup handler registered",
		},
		{
			name:       "no action handler",
			vars:       varsWith(map[string]string{envActionID: "show"}),
			handlers:   noHandlers,
			wantStderr: "no action handler registered",
		},
		{
			name:       "no event handler",
			vars:       varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON}),
			handlers:   noHandlers,
			wantStderr: "no event handler registered",
		},
		{
			name:       "no pane handler",
			vars:       varsWith(map[string]string{envEntrypointID: "board"}),
			handlers:   noHandlers,
			wantStderr: "no pane handler registered",
		},
		{
			name:       "event hook without an envelope",
			vars:       varsWith(map[string]string{envEvent: "pane.agent_status_changed"}),
			handlers:   allHandlers,
			wantStderr: "HERDR_PLUGIN_EVENT_JSON is not set",
		},
		{
			name:       "event hook with a malformed envelope",
			vars:       varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: `{"event":`}),
			handlers:   allHandlers,
			wantStderr: "decode HERDR_PLUGIN_EVENT_JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got calls
			var stderr bytes.Buffer

			code := run(context.Background(), tt.handlers(&got).bind, lookupFrom(tt.vars), &stderr)

			if code != ExitRuntimeError {
				t.Fatalf("run() = %d, want %d", code, ExitRuntimeError)
			}
			if got.kind != "" {
				t.Errorf("handler %q ran, want none", got.kind)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunUsesProcessEnvironment(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PLUGIN_ID", "example.agent-status")
	t.Setenv("HERDR_PLUGIN_ACTION_ID", "show")
	var got calls

	code := Run(context.Background(), handlersRecording(&got, nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d", code, ExitOK)
	}
	if got.kind != KindAction || got.id != "show" {
		t.Errorf("handler = %q, id = %q", got.kind, got.id)
	}
}

func TestExitCodesAreDistinct(t *testing.T) {
	if ExitOK == ExitHandlerError || ExitOK == ExitRuntimeError || ExitHandlerError == ExitRuntimeError {
		t.Fatalf("exit codes overlap: %d %d %d", ExitOK, ExitHandlerError, ExitRuntimeError)
	}
	if ExitOK != 0 {
		t.Errorf("ExitOK = %d, want 0", ExitOK)
	}
}
