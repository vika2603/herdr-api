//go:build e2e

package e2e

import "testing"

// state carries the fixtures one stage creates and a later stage needs.
type state struct {
	workspaceID string
	tabID       string
	paneID      string
	splitPaneID string
}

// TestAPISurface calls every reachable method in one ordered run: the stages
// build on each other's fixtures, so they are subtests of a single test
// rather than independent test functions.
func TestAPISurface(t *testing.T) {
	stages := []struct {
		name string
		run  func(*testing.T, *harness, *state)
	}{
		{"server", stageServer},
		{"workspace", stageWorkspace},
		{"tab", stageTab},
		{"pane", stagePane},
		{"pane-io", stagePaneIO},
		{"layout", stageLayout},
		{"agent", stageAgent},
	}

	var st state
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) { stage.run(t, suite, &st) })
	}
}
