package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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

func TestIsKnownEventName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "worktree.created", want: true},
		{name: "pane.agent_status_changed", want: true},
		{name: "layout.updated", want: true},
		{name: "pane.exploded"},
		{name: "pane_created"},
		{name: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isKnownEventName(tt.name); got != tt.want {
				t.Errorf("isKnownEventName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestKnownEventKindsMatchSchema keeps the local list in step with the schema
// snapshot until the generated EventKind lands.
func TestKnownEventKindsMatchSchema(t *testing.T) {
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
	if !reflect.DeepEqual(knownEventKinds, want) {
		t.Errorf("knownEventKinds =\n%q\nwant\n%q", knownEventKinds, want)
	}
}
