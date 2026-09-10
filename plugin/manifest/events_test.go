package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// hookExcludedKinds are the EventKind values herdr deliberately keeps out of
// PLUGIN_HOOK_EVENT_KINDS, so a manifest hook on one of them is a warning.
var hookExcludedKinds = []string{
	"workspace_metadata_updated",
	"pane_updated",
	"pane_output_changed",
	"layout_updated",
}

func TestDotName(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{kind: "pane_created", want: "pane.created"},
		{kind: "pane_agent_status_changed", want: "pane.agent_status_changed"},
		{kind: "workspace_metadata_updated", want: "workspace.metadata_updated"},
		{kind: "layout_updated", want: "layout.updated"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := dotName(tt.kind); got != tt.want {
				t.Errorf("dotName(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestIsHookEventName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "worktree.created", want: true},
		{name: "pane.agent_status_changed", want: true},
		{name: "workspace.updated", want: true},
		// Known to the socket API but excluded from manifest hooks.
		{name: "pane.output_changed"},
		{name: "pane.updated"},
		{name: "workspace.metadata_updated"},
		{name: "layout.updated"},
		{name: "pane.exploded"},
		{name: "pane_created"},
		{name: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHookEventName(tt.name); got != tt.want {
				t.Errorf("isHookEventName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestHookEventKindsAreSchemaKindsMinusExcluded ties the two lists together:
// every hook event is an EventKind, and the difference is exactly the four
// events herdr excludes.
func TestHookEventKindsAreSchemaKindsMinusExcluded(t *testing.T) {
	excluded := make(map[string]bool, len(hookExcludedKinds))
	for _, kind := range hookExcludedKinds {
		excluded[kind] = true
	}
	var want []string
	for _, kind := range schemaEventKinds {
		if !excluded[kind] {
			want = append(want, kind)
		}
	}
	if !reflect.DeepEqual(hookEventKinds, want) {
		t.Errorf("hookEventKinds =\n%q\nwant\n%q", hookEventKinds, want)
	}
	if len(hookEventKinds) != 22 {
		t.Errorf("hookEventKinds has %d entries, want 22", len(hookEventKinds))
	}
	for _, kind := range hookExcludedKinds {
		if isHookEventName(dotName(kind)) {
			t.Errorf("isHookEventName(%q) = true, want false", dotName(kind))
		}
	}
}

// TestSchemaEventKindsMatchSchema keeps the local list in step with the schema
// snapshot until the generated EventKind lands.
func TestSchemaEventKindsMatchSchema(t *testing.T) {
	path := filepath.Join("..", "..", "schema", "herdr-api.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Schemas struct {
			Event struct {
				Defs struct {
					EventKind struct {
						Enum []string `json:"enum"`
					} `json:"EventKind"`
				} `json:"$defs"`
			} `json:"event"`
		} `json:"schemas"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	want := schema.Schemas.Event.Defs.EventKind.Enum
	if len(want) != 26 {
		t.Fatalf("schema EventKind has %d values, want 26", len(want))
	}
	if !reflect.DeepEqual(schemaEventKinds, want) {
		t.Errorf("schemaEventKinds =\n%q\nwant\n%q", schemaEventKinds, want)
	}
}

func TestHookEventNames(t *testing.T) {
	got := HookEventNames()

	want := make([]string, len(hookEventKinds))
	for i, kind := range hookEventKinds {
		want[i] = dotName(kind)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HookEventNames() =\n%q\nwant\n%q", got, want)
	}
	for _, name := range got {
		if !isHookEventName(name) {
			t.Errorf("HookEventNames() returned %q, which isHookEventName rejects", name)
		}
	}
	for _, kind := range hookExcludedKinds {
		if slices.Contains(got, dotName(kind)) {
			t.Errorf("HookEventNames() contains %q, which herdr never fires a hook for", dotName(kind))
		}
	}

	got[0] = "mutated"
	if HookEventNames()[0] == "mutated" {
		t.Error("HookEventNames() shares its backing array between calls")
	}
}
