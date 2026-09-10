package plugintest_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client"
	"github.com/vika2603/herdr-client/plugin"
	"github.com/vika2603/herdr-client/plugin/plugintest"
)

// recorder captures what CheckManifest reports. testing.TB cannot be
// implemented outside the testing package, so it is embedded and the two
// methods CheckManifest calls are overridden; Fatalf unwinds through a panic,
// as t.Fatalf unwinds through runtime.Goexit.
type recorder struct {
	testing.TB
	errors []string
	fatal  string
}

type fatalPanic struct{}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = sprintf(format, args...)
	panic(fatalPanic{})
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// check runs CheckManifest against a recorder and returns what it reported.
func check(t *testing.T, path string, p *plugin.Plugin) (rec *recorder) {
	t.Helper()
	rec = &recorder{TB: t}
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(fatalPanic); !ok {
				panic(recovered)
			}
		}
	}()
	plugintest.CheckManifest(rec, path, p)
	return rec
}

// servedPlugin serves every entrypoint testdata/matching.toml declares.
func servedPlugin() *plugin.Plugin {
	p := plugin.New()
	p.Startup(noop)
	p.Action("show", noop)
	p.Action("clear", noop)
	p.Pane("board", noop)
	plugin.OnEvent(p, func(context.Context, *plugin.Env, *herdr.PaneAgentStatusChangedEvent) error { return nil })
	return p
}

func noop(context.Context, *plugin.Env) error { return nil }

func TestCheckManifestAccepts(t *testing.T) {
	rec := check(t, filepath.Join("testdata", "matching.toml"), servedPlugin())

	if len(rec.errors) != 0 || rec.fatal != "" {
		t.Errorf("errors = %v, fatal = %q, want none", rec.errors, rec.fatal)
	}
}

func TestCheckManifestReportsDisagreements(t *testing.T) {
	p := plugin.New()
	p.Action("hide", noop)
	plugin.OnEvent(p, func(context.Context, *plugin.Env, *herdr.PaneClosedEvent) error { return nil })

	rec := check(t, filepath.Join("testdata", "unserved.toml"), p)

	want := []string{
		`action "show" is declared but no handler is registered for it`,
		`a handler is registered for action "hide" but the manifest declares none`,
		`pane "board" is declared but no handler is registered for it`,
		`event hook "pane.created" is declared but no handler is registered for it`,
		`a handler is registered for event hook "pane.closed" but the manifest declares none`,
	}
	assertReported(t, rec, want)
}

// Both plugins below agree with their manifest except over the startup hook,
// so that the startup report is the only one.
func TestCheckManifestReportsStartupBothWays(t *testing.T) {
	withHandler := plugin.New()
	withHandler.Startup(noop)
	withHandler.Action("show", noop)
	withHandler.Pane("board", noop)
	plugin.OnEvent(withHandler, func(context.Context, *plugin.Env, *herdr.PaneCreatedEvent) error { return nil })
	rec := check(t, filepath.Join("testdata", "unserved.toml"), withHandler)
	assertReported(t, rec, []string{"a startup handler is registered but the manifest declares no [[startup]] entry"})

	withoutHandler := plugin.New()
	withoutHandler.Action("show", noop)
	withoutHandler.Action("clear", noop)
	withoutHandler.Pane("board", noop)
	plugin.OnEvent(withoutHandler, func(context.Context, *plugin.Env, *herdr.PaneAgentStatusChangedEvent) error { return nil })
	rec = check(t, filepath.Join("testdata", "matching.toml"), withoutHandler)
	assertReported(t, rec, []string{"a [[startup]] entry is declared but no startup handler is registered"})
}

// herdr never fires a hook for pane.updated, so naming it is a warning there
// and reported here even though a handler is registered for it.
func TestCheckManifestReportsAnEventHerdrNeverFires(t *testing.T) {
	p := plugin.New()
	plugin.OnEvent(p, func(context.Context, *plugin.Env, *herdr.PaneUpdatedEvent) error { return nil })

	rec := check(t, filepath.Join("testdata", "non-hook-event.toml"), p)

	assertReported(t, rec, []string{"unknown event 'pane.updated'"})
	for _, reported := range rec.errors {
		if strings.Contains(reported, "no handler is registered") {
			t.Errorf("reported %q, but a handler is registered for it", reported)
		}
	}
}

func TestCheckManifestReportsWarnings(t *testing.T) {
	rec := check(t, filepath.Join("testdata", "no-platforms.toml"), plugin.New())

	assertReported(t, rec, []string{"does not declare platforms"})
}

func TestCheckManifestStopsOnAManifestItCannotUse(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "invalid", path: filepath.Join("testdata", "invalid.toml"), want: "invalid_plugin_action_title"},
		{name: "absent", path: filepath.Join("testdata", "absent.toml"), want: "absent.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := check(t, tt.path, plugin.New())
			if !strings.Contains(rec.fatal, tt.want) {
				t.Errorf("fatal = %q, want it to contain %q", rec.fatal, tt.want)
			}
			if len(rec.errors) != 0 {
				t.Errorf("errors = %v, want none before the manifest is usable", rec.errors)
			}
		})
	}
}

func assertReported(t *testing.T, rec *recorder, want []string) {
	t.Helper()
	for _, fragment := range want {
		found := false
		for _, reported := range rec.errors {
			if strings.Contains(reported, fragment) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("reported %v, want one of them to contain %q", rec.errors, fragment)
		}
	}
	if len(rec.errors) != len(want) {
		t.Errorf("reported %d problems, want %d: %v", len(rec.errors), len(want), rec.errors)
	}
}

// A handler for an event Herdr never hooks cannot be reached by any manifest
// entry, so the report names that instead of a missing declaration.
func TestCheckManifestReportsAnUnreachableEventHandler(t *testing.T) {
	p := plugin.New()
	plugin.OnEvent(p, func(context.Context, *plugin.Env, *herdr.LayoutUpdatedEvent) error { return nil })

	rec := check(t, filepath.Join("testdata", "no-entrypoints.toml"), p)

	assertReported(t, rec, []string{`event "layout.updated", which Herdr never fires a plugin hook for`})
}
