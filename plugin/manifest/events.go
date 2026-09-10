package manifest

import "strings"

// knownEventKinds is the EventKind enum of the herdr 0.9.0 API schema
// (schema/herdr-api.schema.json) in underscore form. It stays local to this
// package until the generated EventKind lands.
var knownEventKinds = []string{
	"workspace_created",
	"workspace_updated",
	"workspace_metadata_updated",
	"workspace_closed",
	"workspace_renamed",
	"workspace_moved",
	"workspace_reordered",
	"workspace_focused",
	"worktree_created",
	"worktree_opened",
	"worktree_removed",
	"tab_created",
	"tab_closed",
	"tab_renamed",
	"tab_moved",
	"tab_focused",
	"pane_created",
	"pane_closed",
	"pane_updated",
	"pane_focused",
	"pane_moved",
	"pane_output_changed",
	"pane_exited",
	"pane_agent_detected",
	"pane_agent_status_changed",
	"layout_updated",
}

// dotName converts an EventKind to the dotted name a manifest event hook
// refers to, matching EventKind::dot_name in herdr.
func dotName(kind string) string { return strings.Replace(kind, "_", ".", 1) }

// isKnownEventName reports whether name is the dotted form of a known
// EventKind.
func isKnownEventName(name string) bool {
	for _, kind := range knownEventKinds {
		if dotName(kind) == name {
			return true
		}
	}
	return false
}
