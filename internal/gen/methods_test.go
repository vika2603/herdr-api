package gen

import (
	"strings"
	"testing"
)

func mustParseTable(t *testing.T, source string) *MethodTable {
	t.Helper()
	table, err := ParseMethodTable([]byte(source))
	if err != nil {
		t.Fatalf("ParseMethodTable: %v", err)
	}
	return table
}

func TestMethodTableShapes(t *testing.T) {
	table := mustParseTable(t, `{"methods": {
		"ping": {"result": "pong"},
		"events.subscribe": {"stream": "subscription_started"},
		"plugin.pane.open": {"results": ["plugin_pane_opened", "ok"]},
		"command.invoke": {"results": ["*"]}
	}}`)
	if got := table.Methods["ping"].Result; got != "pong" {
		t.Errorf("ping result = %q, want pong", got)
	}
	if got := table.Methods["events.subscribe"].Stream; got != "subscription_started" {
		t.Errorf("events.subscribe stream = %q", got)
	}
	if got := table.Methods["plugin.pane.open"].Results; len(got) != 2 {
		t.Errorf("plugin.pane.open results = %v", got)
	}
}

func TestMethodTableRejectsAmbiguousEntries(t *testing.T) {
	_, err := ParseMethodTable([]byte(`{"methods": {"ping": {"result": "pong", "stream": "pong"}}}`))
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error = %v, want it to reject two shapes", err)
	}
	_, err = ParseMethodTable([]byte(`{"methods": {"ping": {"returns": "pong"}}}`))
	if err == nil || !strings.Contains(err.Error(), "returns") {
		t.Fatalf("error = %v, want it to name the unsupported key", err)
	}
}

func TestMethodTableValidation(t *testing.T) {
	tags := map[string]bool{"pong": true, "ok": true}

	table := mustParseTable(t, `{"methods": {"ping": {"result": "pong"}}}`)
	if err := table.validate([]string{"ping", "server.stop"}, tags); err == nil ||
		!strings.Contains(err.Error(), "server.stop") {
		t.Fatalf("error = %v, want it to report the missing method", err)
	}

	table = mustParseTable(t, `{"methods": {"ping": {"result": "pong"}, "gone": {"result": "ok"}}}`)
	if err := table.validate([]string{"ping"}, tags); err == nil ||
		!strings.Contains(err.Error(), "gone") {
		t.Fatalf("error = %v, want it to report the unknown method", err)
	}

	table = mustParseTable(t, `{"methods": {"ping": {"result": "pang"}}}`)
	if err := table.validate([]string{"ping"}, tags); err == nil ||
		!strings.Contains(err.Error(), "pang") {
		t.Fatalf("error = %v, want it to report the unknown result type", err)
	}

	table = mustParseTable(t, `{"methods": {"ping": {"result": "pong"}}}`)
	if err := table.validate([]string{"ping"}, tags); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
