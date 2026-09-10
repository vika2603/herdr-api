package plugin

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/herdr"
)

// recordRun returns a handler that records that it ran and returns err.
func recordRun(ran *string, name string, err error) func(context.Context, *Env) error {
	return func(_ context.Context, _ *Env) error {
		*ran = name
		return err
	}
}

func TestPluginDispatchesByID(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{name: "startup", vars: map[string]string{envEvent: "startup"}, want: "startup"},
		{name: "action", vars: map[string]string{envActionID: "show"}, want: "action show"},
		{name: "second action", vars: map[string]string{envActionID: "clear"}, want: "action clear"},
		{name: "pane", vars: map[string]string{envEntrypointID: "board"}, want: "pane board"},
		{
			name: "event",
			vars: map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON},
			want: "event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ran string
			p := New()
			p.Startup(recordRun(&ran, "startup", nil))
			p.Action("show", recordRun(&ran, "action show", nil))
			p.Action("clear", recordRun(&ran, "action clear", nil))
			p.Pane("board", recordRun(&ran, "pane board", nil))
			OnEvent(p, func(_ context.Context, _ *Env, _ *herdr.PaneAgentStatusChangedEvent) error {
				ran = "event"
				return nil
			})

			var stderr strings.Builder
			code := run(context.Background(), p.bind, lookupFrom(varsWith(tt.vars)), &stderr)
			if code != ExitOK {
				t.Fatalf("run() = %d, want %d (stderr %q)", code, ExitOK, stderr.String())
			}
			if ran != tt.want {
				t.Errorf("ran %q, want %q", ran, tt.want)
			}
		})
	}
}

func TestPluginPassesTypedEventPayload(t *testing.T) {
	var got *herdr.PaneAgentStatusChangedEvent
	p := New()
	OnEvent(p, func(_ context.Context, _ *Env, e *herdr.PaneAgentStatusChangedEvent) error {
		got = e
		return nil
	})

	vars := varsWith(map[string]string{envEvent: "pane.agent_status_changed", envEventJSON: statusEventJSON})
	if code := run(context.Background(), p.bind, lookupFrom(vars), &strings.Builder{}); code != ExitOK {
		t.Fatalf("run() = %d, want %d", code, ExitOK)
	}
	if got == nil || got.PaneID != "pane-1" || got.AgentStatus != herdr.AgentStatusDone {
		t.Errorf("payload = %+v", got)
	}
}

func TestPluginHandlerErrorKeepsTheExitContract(t *testing.T) {
	var ran string
	p := New()
	p.Action("show", recordRun(&ran, "action show", errors.New("boom")))

	var stderr strings.Builder
	vars := varsWith(map[string]string{envActionID: "show"})
	if code := run(context.Background(), p.bind, lookupFrom(vars), &stderr); code != ExitHandlerError {
		t.Fatalf("run() = %d, want %d", code, ExitHandlerError)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want the handler error", stderr.String())
	}
}

func TestPluginUnregisteredIDIsARuntimeError(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{name: "unknown action", vars: map[string]string{envActionID: "hide"}, want: `no action handler registered for "hide"`},
		{name: "unknown pane", vars: map[string]string{envEntrypointID: "other"}, want: `no pane handler registered for "other"`},
		{
			name: "unknown event",
			vars: map[string]string{envEvent: "pane.created", envEventJSON: `{"event":"pane_created","data":{"type":"pane_created","pane":{}}}`},
			want: `no event handler registered for "pane.created"`,
		},
		{name: "no startup handler", vars: map[string]string{envEvent: "startup"}, want: "no startup handler registered"},
		{name: "no entrypoint marker", vars: nil, want: "entrypoint kind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			p.Action("show", func(context.Context, *Env) error { return nil })
			p.Pane("board", func(context.Context, *Env) error { return nil })

			var stderr strings.Builder
			code := run(context.Background(), p.bind, lookupFrom(varsWith(tt.vars)), &stderr)
			if code != ExitRuntimeError {
				t.Fatalf("run() = %d, want %d", code, ExitRuntimeError)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

// The event name a hook is invoked for selects the handler, so a payload that
// does not match the registered type must not reach it.
func TestPluginRejectsAMismatchedPayload(t *testing.T) {
	p := New()
	OnEvent(p, func(context.Context, *Env, *herdr.PaneCreatedEvent) error {
		t.Error("handler ran with a payload of another type")
		return nil
	})

	var stderr strings.Builder
	vars := varsWith(map[string]string{
		envEvent:     "pane.created",
		envEventJSON: `{"event":"pane_created","data":{"type":"pane_closed","pane_id":"pane-1","workspace_id":"ws-1"}}`,
	})
	if code := run(context.Background(), p.bind, lookupFrom(vars), &stderr); code != ExitRuntimeError {
		t.Fatalf("run() = %d, want %d", code, ExitRuntimeError)
	}
	if !strings.Contains(stderr.String(), "want *herdr.PaneCreatedEvent") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestOnEventTakesTheNameFromThePayloadType(t *testing.T) {
	p := New()
	OnEvent(p, func(context.Context, *Env, *herdr.PaneCreatedEvent) error { return nil })
	OnEvent(p, func(context.Context, *Env, *herdr.WorktreeCreatedEvent) error { return nil })
	// Shared between the lifecycle envelope and the dedicated subscription;
	// both spell the hook name pane.agent_status_changed.
	OnEvent(p, func(context.Context, *Env, *herdr.PaneAgentStatusChangedEvent) error { return nil })

	want := []string{"pane.agent_status_changed", "pane.created", "worktree.created"}
	if got := p.Registered().Events; !reflect.DeepEqual(got, want) {
		t.Errorf("Events = %v, want %v", got, want)
	}
}

func TestRegistered(t *testing.T) {
	p := New()
	p.Action("show", func(context.Context, *Env) error { return nil })
	p.Action("clear", func(context.Context, *Env) error { return nil })
	p.Pane("board", func(context.Context, *Env) error { return nil })

	got := p.Registered()
	want := Registered{Actions: []string{"clear", "show"}, Panes: []string{"board"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Registered() = %+v, want %+v", got, want)
	}

	p.Startup(func(context.Context, *Env) error { return nil })
	if !p.Registered().Startup {
		t.Error("Startup = false after registering a startup handler")
	}
}

func TestRegistrationMistakesPanic(t *testing.T) {
	tests := []struct {
		name     string
		register func(*Plugin)
		want     string
	}{
		{
			name:     "duplicate action",
			register: func(p *Plugin) { p.Action("show", noop); p.Action("show", noop) },
			want:     `action "show" is already registered`,
		},
		{
			name:     "duplicate pane",
			register: func(p *Plugin) { p.Pane("board", noop); p.Pane("board", noop) },
			want:     `pane "board" is already registered`,
		},
		{
			name:     "duplicate startup",
			register: func(p *Plugin) { p.Startup(noop); p.Startup(noop) },
			want:     "startup hook is already registered",
		},
		{
			name: "duplicate event",
			register: func(p *Plugin) {
				OnEvent(p, func(context.Context, *Env, *herdr.PaneCreatedEvent) error { return nil })
				OnEvent(p, func(context.Context, *Env, *herdr.PaneCreatedEvent) error { return nil })
			},
			want: "event pane.created is already registered",
		},
		{name: "empty action id", register: func(p *Plugin) { p.Action("", noop) }, want: "action id is empty"},
		{name: "nil action handler", register: func(p *Plugin) { p.Action("show", nil) }, want: "nil handler for action show"},
		{name: "nil startup handler", register: func(p *Plugin) { p.Startup(nil) }, want: "nil handler for startup hook"},
		{
			name: "nil event handler",
			register: func(p *Plugin) {
				OnEvent[herdr.PaneCreatedEvent](p, nil)
			},
			want: "nil handler for event hook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				recovered, _ := recover().(string)
				if !strings.Contains(recovered, tt.want) {
					t.Errorf("recover() = %q, want it to contain %q", recovered, tt.want)
				}
			}()
			tt.register(New())
			t.Error("registration did not panic")
		})
	}
}

func noop(context.Context, *Env) error { return nil }

func TestPluginDispatchRunsAHandlerWithoutTheProcessEnvironment(t *testing.T) {
	var ran string
	p := New()
	p.Action("show", recordRun(&ran, "action show", nil))
	OnEvent(p, func(_ context.Context, _ *Env, e *herdr.PaneAgentStatusChangedEvent) error {
		ran = "event " + e.PaneID
		return nil
	})

	tests := []struct {
		name string
		env  *Env
		want string
	}{
		{name: "action", env: &Env{ActionID: "show"}, want: "action show"},
		{
			name: "event",
			env:  &Env{Event: "pane.agent_status_changed", EventJSON: []byte(statusEventJSON)},
			want: "event pane-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran = ""
			if err := p.Dispatch(context.Background(), tt.env); err != nil {
				t.Fatalf("Dispatch() = %v, want nil", err)
			}
			if ran != tt.want {
				t.Errorf("ran %q, want %q", ran, tt.want)
			}
		})
	}
}

func TestPluginDispatchReturnsTheHandlerError(t *testing.T) {
	want := errors.New("boom")
	p := New()
	p.Action("show", func(context.Context, *Env) error { return want })

	if err := p.Dispatch(context.Background(), &Env{ActionID: "show"}); !errors.Is(err, want) {
		t.Errorf("Dispatch() = %v, want %v", err, want)
	}
}

func TestPluginDispatchReportsThatNoHandlerRan(t *testing.T) {
	tests := []struct {
		name string
		env  *Env
		want string
	}{
		{name: "unregistered action", env: &Env{ActionID: "hide"}, want: `no action handler registered for "hide"`},
		{name: "no entrypoint marker", env: &Env{}, want: "entrypoint kind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			p.Action("show", noop)

			err := p.Dispatch(context.Background(), tt.env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Dispatch() = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
