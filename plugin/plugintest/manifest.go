package plugintest

import (
	"slices"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/plugin"
	"github.com/vika2603/herdr-client/plugin/manifest"
)

// CheckManifest reports every disagreement between the manifest at path and
// the entrypoints p serves:
//
//   - an id or event the manifest declares that no handler is registered for,
//     which Herdr would only report when the user invokes it;
//   - a handler with no manifest entry, which nothing can ever reach;
//   - a handler registered for an event Herdr never fires a plugin hook for,
//     which no manifest entry could reach either;
//   - every rule herdr enforces when it loads the manifest, and every warning
//     it prints, which includes an [[events]] hook naming an event herdr
//     never fires a hook for and a manifest that declares no platforms.
//
// A manifest that does not parse or does not validate stops the test, since
// nothing can be compared against it.
//
//	func TestManifest(t *testing.T) {
//		plugintest.CheckManifest(t, "herdr-plugin.toml", plugin.New())
//	}
func CheckManifest(t testing.TB, path string, p *plugin.Plugin) {
	t.Helper()

	parsed, warnings, err := manifest.Parse(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for _, warning := range warnings {
		t.Errorf("%s: %s", path, warning)
	}

	registered := p.Registered()
	compare(t, path, "action", declaredActions(parsed), registered.Actions)
	compare(t, path, "pane", declaredPanes(parsed), registered.Panes)
	compareEvents(t, path, declaredEvents(parsed), registered.Events)

	switch {
	case len(parsed.Startup) > 0 && !registered.Startup:
		t.Errorf("%s: a [[startup]] entry is declared but no startup handler is registered", path)
	case len(parsed.Startup) == 0 && registered.Startup:
		t.Errorf("%s: a startup handler is registered but the manifest declares no [[startup]] entry", path)
	}
}

// compare reports the ids on one side and not the other. Both sides are
// sorted so that the report is stable.
func compare(t testing.TB, path, what string, declared, registered []string) {
	t.Helper()

	slices.Sort(declared)
	for _, id := range declared {
		if !slices.Contains(registered, id) {
			t.Errorf("%s: %s %q is declared but no handler is registered for it", path, what, id)
		}
	}
	for _, id := range registered {
		if !slices.Contains(declared, id) {
			t.Errorf("%s: a handler is registered for %s %q but the manifest declares none", path, what, id)
		}
	}
}

// compareEvents is compare for event hooks, with the extra case that a
// handler can be registered for an event Herdr never fires a hook for. Such a
// handler can never run, and no manifest entry would reach it either, so
// reporting it as a missing manifest entry would point at the wrong fix.
//
// An undeliverable event the manifest does declare is left to Validate, which
// already warns about it; repeating it here would report one defect twice.
func compareEvents(t testing.TB, path string, declared, registered []string) {
	t.Helper()

	hookable := make(map[string]bool)
	for _, name := range manifest.HookEventNames() {
		hookable[name] = true
	}

	slices.Sort(declared)
	for _, name := range declared {
		if !slices.Contains(registered, name) {
			t.Errorf("%s: event hook %q is declared but no handler is registered for it", path, name)
		}
	}
	for _, name := range registered {
		switch {
		case slices.Contains(declared, name):
		case !hookable[name]:
			t.Errorf("%s: a handler is registered for event %q, which Herdr never fires a plugin hook for", path, name)
		default:
			t.Errorf("%s: a handler is registered for event hook %q but the manifest declares none", path, name)
		}
	}
}

// The declared* functions read the ids the way herdr compares them, with
// surrounding whitespace trimmed.

func declaredActions(m *manifest.Manifest) []string {
	ids := make([]string, 0, len(m.Actions))
	for _, action := range m.Actions {
		ids = append(ids, strings.TrimSpace(action.ID))
	}
	return ids
}

func declaredPanes(m *manifest.Manifest) []string {
	ids := make([]string, 0, len(m.Panes))
	for _, pane := range m.Panes {
		ids = append(ids, strings.TrimSpace(pane.ID))
	}
	return ids
}

func declaredEvents(m *manifest.Manifest) []string {
	names := make([]string, 0, len(m.Events))
	for _, hook := range m.Events {
		names = append(names, strings.TrimSpace(hook.On))
	}
	return names
}
