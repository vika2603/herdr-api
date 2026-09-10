//go:build e2e

package e2e

import (
	"testing"

	"github.com/vika2603/herdr-client"
)

// stagePane splits the workspace root pane and calls the structural pane
// methods on the pair. The split pane stays for the layout and teardown
// stages.
func stagePane(t *testing.T, h *harness, st *state) {
	split, err := h.client.PaneSplit(h.ctx(t), herdr.PaneSplitParams{
		TargetPaneID: ptr(st.paneID),
		Direction:    herdr.SplitDirectionRight,
		Cwd:          ptr(h.root),
		Focus:        ptr(false),
	})
	if !h.cover(t, herdr.MethodPaneSplit, split, err) {
		t.Fatal("no second pane to continue with")
	}
	st.splitPaneID = split.Pane.PaneID
	if st.splitPaneID == "" || split.Pane.TabID != st.tabID {
		t.Fatalf("pane.split returned %+v, expected a pane in %s", split.Pane, st.tabID)
	}

	info, err := h.client.PaneGet(h.ctx(t), herdr.PaneTarget{PaneID: st.paneID})
	if h.cover(t, herdr.MethodPaneGet, info, err) && info.Pane.PaneID != st.paneID {
		t.Errorf("pane.get returned %s, asked for %s", info.Pane.PaneID, st.paneID)
	}

	focused, err := h.client.PaneFocus(h.ctx(t), herdr.PaneTarget{PaneID: st.paneID})
	if h.cover(t, herdr.MethodPaneFocus, focused, err) && !focused.Pane.Focused {
		t.Errorf("pane.focus left %s unfocused", st.paneID)
	}

	list, err := h.client.PaneList(h.ctx(t), herdr.PaneListParams{WorkspaceID: ptr(st.workspaceID)})
	if h.cover(t, herdr.MethodPaneList, list, err) {
		seen := make(map[string]bool, len(list.Panes))
		for _, pane := range list.Panes {
			seen[pane.PaneID] = true
		}
		if !seen[st.paneID] || !seen[st.splitPaneID] {
			t.Errorf("pane.list is missing %s or %s: %+v", st.paneID, st.splitPaneID, list.Panes)
		}
	}

	current, err := h.client.PaneCurrent(h.ctx(t), herdr.PaneCurrentParams{CallerPaneID: ptr(st.paneID)})
	if h.cover(t, herdr.MethodPaneCurrent, current, err) && current.Pane.PaneID == "" {
		t.Errorf("pane.current returned no pane")
	}

	renamed, err := h.client.PaneRename(h.ctx(t), herdr.PaneRenameParams{
		PaneID: st.paneID,
		Label:  ptr("e2e-pane"),
	})
	if h.cover(t, herdr.MethodPaneRename, renamed, err) {
		if renamed.Pane.Label == nil || *renamed.Pane.Label != "e2e-pane" {
			t.Errorf("pane.rename reported label %v", renamed.Pane.Label)
		}
	}

	scrolled, err := h.client.PaneScroll(h.ctx(t), herdr.PaneScrollParams{
		PaneID:           st.paneID,
		OffsetFromBottom: 0,
	})
	if h.cover(t, herdr.MethodPaneScroll, scrolled, err) && scrolled.Pane.Scroll == nil {
		t.Errorf("pane.scroll reported no scroll state")
	}

	input, err := h.client.PaneInputSet(h.ctx(t), herdr.PaneInputSetParams{
		PaneID:     st.paneID,
		RightClick: herdr.PaneRightClickTargetPane,
	})
	h.cover(t, herdr.MethodPaneInputSet, input, err)

	neighbor, err := h.client.PaneNeighbor(h.ctx(t), herdr.PaneNeighborParams{
		PaneID:    ptr(st.paneID),
		Direction: herdr.PaneDirectionRight,
	})
	if h.cover(t, herdr.MethodPaneNeighbor, neighbor, err) {
		if neighbor.Neighbor.NeighborPaneID == nil || *neighbor.Neighbor.NeighborPaneID != st.splitPaneID {
			t.Errorf("pane.neighbor to the right of %s is %v, expected %s",
				st.paneID, neighbor.Neighbor.NeighborPaneID, st.splitPaneID)
		}
	}

	edges, err := h.client.PaneEdges(h.ctx(t), herdr.PaneEdgesParams{PaneID: ptr(st.paneID)})
	if h.cover(t, herdr.MethodPaneEdges, edges, err) && edges.Edges.Right {
		t.Errorf("pane.edges reports %s at the right edge although it was split to the right", st.paneID)
	}

	resized, err := h.client.PaneResize(h.ctx(t), herdr.PaneResizeParams{
		PaneID:    ptr(st.paneID),
		Direction: herdr.PaneDirectionRight,
		Amount:    ptr(0.05),
	})
	if h.cover(t, herdr.MethodPaneResize, resized, err) && !resized.Resize.Changed {
		t.Errorf("pane.resize changed nothing: %+v", resized.Resize)
	}

	direction, err := h.client.PaneFocusDirection(h.ctx(t), herdr.PaneFocusDirectionParams{
		PaneID:    ptr(st.paneID),
		Direction: herdr.PaneDirectionRight,
	})
	if h.cover(t, herdr.MethodPaneFocusDirection, direction, err) {
		if direction.Focus.FocusedPaneID == nil || *direction.Focus.FocusedPaneID != st.splitPaneID {
			t.Errorf("pane.focus_direction focused %v, expected %s",
				direction.Focus.FocusedPaneID, st.splitPaneID)
		}
	}

	swapped, err := h.client.PaneSwap(h.ctx(t), herdr.PaneSwapParams{
		SourcePaneID: ptr(st.paneID),
		TargetPaneID: ptr(st.splitPaneID),
	})
	if h.cover(t, herdr.MethodPaneSwap, swapped, err) && !swapped.Swap.Changed {
		t.Errorf("pane.swap changed nothing: %+v", swapped.Swap)
	}

	zoomed, err := h.client.PaneZoom(h.ctx(t), herdr.PaneZoomParams{
		PaneID: ptr(st.paneID),
		Mode:   herdr.PaneZoomModeOn,
	})
	if h.cover(t, herdr.MethodPaneZoom, zoomed, err) && !zoomed.Zoom.Zoomed {
		t.Errorf("pane.zoom on left %s unzoomed", st.paneID)
	}
	if _, err := h.client.PaneZoom(h.ctx(t), herdr.PaneZoomParams{
		PaneID: ptr(st.paneID),
		Mode:   herdr.PaneZoomModeOff,
	}); err != nil {
		t.Fatalf("unzoom %s: %v", st.paneID, err)
	}

	layout, err := h.client.PaneLayout(h.ctx(t), herdr.PaneLayoutParams{PaneID: ptr(st.paneID)})
	if h.cover(t, herdr.MethodPaneLayout, layout, err) && len(layout.Layout.Panes) != 2 {
		t.Errorf("pane.layout reports %d panes, expected 2", len(layout.Layout.Panes))
	}

	process, err := h.client.PaneProcessInfo(h.ctx(t), herdr.PaneProcessInfoParams{PaneID: ptr(st.paneID)})
	if h.cover(t, herdr.MethodPaneProcessInfo, process, err) {
		if process.ProcessInfo.ShellPID == nil || *process.ProcessInfo.ShellPID == 0 {
			t.Errorf("pane.process_info reported no shell pid: %+v", process.ProcessInfo)
		}
	}

	// pane.link.activate resolves a cell of the rendered pane, so the pane
	// has to be the focused one. There is no link under the cell, which the
	// response reports as not handled.
	if _, err := h.client.PaneFocus(h.ctx(t), herdr.PaneTarget{PaneID: st.paneID}); err != nil {
		t.Fatalf("refocus %s: %v", st.paneID, err)
	}
	link, err := h.client.PaneLinkActivate(h.ctx(t), herdr.PaneLinkActivateParams{
		PaneID:      st.paneID,
		ViewportRow: 0,
		Col:         0,
	})
	if h.cover(t, herdr.MethodPaneLinkActivate, link, err) && link.Handled && link.URL == nil {
		t.Errorf("pane.link.activate reported a handled link without a URL")
	}
}
