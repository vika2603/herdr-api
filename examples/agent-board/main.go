// Command herdr-agent-board is a worked example of a Herdr plugin pane: it
// mirrors the running session and redraws a board of its workspaces, tabs and
// agents on every change until the user closes the pane.
//
// Herdr starts the board from the [[panes]] entrypoint, and the "open" action
// starts it through plugin.pane.open. See README.md.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vika2603/herdr-client"
	"github.com/vika2603/herdr-client/plugin"
)

// Entrypoint ids herdr-plugin.toml declares.
const (
	paneBoard  = "board"
	actionOpen = "open"
)

// boardTitle is the title the board reports for its own pane.
const boardTitle = "agent board"

// clearScreen homes the cursor and clears the screen, so each frame replaces
// the previous one instead of scrolling it out of sight.
const clearScreen = "\x1b[H\x1b[2J"

func main() {
	// A pane entrypoint runs until the user closes the pane, which delivers
	// SIGHUP and then SIGTERM. Without this context the process would be
	// killed in the middle of a frame instead of ending its loop.
	ctx, stop := plugin.ShutdownContext(context.Background())
	defer stop()
	os.Exit(newPlugin().Run(ctx))
}

// newPlugin registers one handler per entrypoint of herdr-plugin.toml.
// TestManifest checks the two against each other.
func newPlugin() *plugin.Plugin {
	p := plugin.New()
	p.Pane(paneBoard, onBoard)
	p.Action(actionOpen, onOpen)
	return p
}

// onOpen opens the board in a pane. Herdr can start a pane entrypoint on its
// own, so this action exists to show the other way in: placement, size and
// title come from the manifest, which leaves the entrypoint id the only thing
// the call has to name.
func onOpen(ctx context.Context, env *plugin.Env) error {
	_, err := env.Client().PluginPaneOpen(ctx, herdr.PluginPaneOpenParams{
		PluginID:   env.PluginID,
		Entrypoint: paneBoard,
		Focus:      herdr.Ptr(true),
	})
	return err
}

// onBoard mirrors the session and redraws the board until the pane closes.
func onBoard(ctx context.Context, env *plugin.Env) error {
	client := env.Client()
	session, err := herdr.OpenSession(ctx, client)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	if err := reportTitle(ctx, client, env); err != nil {
		return err
	}

	frame := board{}.from(session)
	draw(os.Stdout, frame)
	for {
		event, err := session.Next(ctx)
		if err != nil {
			var unknown *herdr.UnknownEventError
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, herdr.ErrStreamClosed):
				// The pane was closed, or the mirror gave up reconnecting.
				// Neither is a failure of the board.
				return nil
			case errors.As(err, &unknown):
				// An event a newer server added. It never reached the mirror,
				// so the board still shows the state it had.
				continue
			default:
				return err
			}
		}
		frame = frame.after(event).from(session)
		draw(os.Stdout, frame)
	}
}

// reportTitle gives the board's own pane the title Herdr displays for it.
// Herdr keeps one title per source, so the plugin id is the source: a later
// report from this plugin replaces the title, and no other reporter can.
//
// A popup plugin pane receives no pane id, and a pane cannot name itself
// without one, so the board runs untitled there.
func reportTitle(ctx context.Context, client *herdr.Client, env *plugin.Env) error {
	if env.PaneID == "" {
		return nil
	}
	_, err := client.PaneReportMetadata(ctx, herdr.PaneReportMetadataParams{
		PaneID: env.PaneID,
		Source: env.PluginID,
		Title:  herdr.Ptr(boardTitle),
	})
	return err
}

// board is one frame: the mirrored state it shows, and the two counters the
// board keeps itself because the mirror holds no history.
type board struct {
	Workspaces []herdr.WorkspaceInfo
	Tabs       []herdr.TabInfo
	Agents     []herdr.AgentInfo
	// Panes holds the pane each agent runs in, for the pane fields AgentInfo
	// does not repeat.
	Panes map[string]herdr.PaneInfo
	// Events counts the events this frame was built from; Note reports a gap
	// in them.
	Events int
	Note   string
}

// from reads the mirror into the frame. The accessors return copies, so the
// frame stays as it was read while the next event is applied.
func (b board) from(s *herdr.Session) board {
	b.Workspaces = s.Workspaces()
	b.Tabs = s.Tabs()
	b.Agents = s.Agents()
	b.Panes = make(map[string]herdr.PaneInfo, len(b.Agents))
	for _, agent := range b.Agents {
		if pane, ok := s.Pane(agent.PaneID); ok {
			b.Panes[agent.PaneID] = pane
		}
	}
	return b
}

// after folds one event into the counters. A ResyncEvent reports that the
// stream ended and the mirror was rebuilt from a fresh snapshot: the events of
// the gap are gone, so the count starts again rather than describing state the
// board never saw arrive.
func (b board) after(event herdr.Event) board {
	b.Events++
	b.Note = ""
	if resync, ok := event.(*herdr.ResyncEvent); ok {
		b.Events = 0
		b.Note = fmt.Sprintf("resynced from a new snapshot after %v", resync.Cause)
	}
	return b
}

// paneLabel names the pane an agent runs in. The label is a pane field that
// AgentInfo does not carry, which is why the frame keeps the panes too; a pane
// without a label, or one the mirror no longer holds, is named by its id.
func (b board) paneLabel(paneID string) string {
	pane, ok := b.Panes[paneID]
	if !ok {
		return paneID
	}
	return cmp.Or(herdr.Value(pane.Label), paneID)
}

// render draws one frame. It reads nothing but the frame, so the whole layout
// is testable without a server.
func render(b board) string {
	var out strings.Builder
	fmt.Fprintf(&out, "agent board  workspaces %d  agents %d  events %d\n",
		len(b.Workspaces), len(b.Agents), b.Events)
	if b.Note != "" {
		fmt.Fprintf(&out, "%s\n", b.Note)
	}
	if len(b.Workspaces) == 0 {
		out.WriteString("\nthe session has no workspaces\n")
		return out.String()
	}
	for _, workspace := range b.Workspaces {
		fmt.Fprintf(&out, "\n%s ws %d %s  tabs %d  panes %d  [%s]\n",
			mark(workspace.Focused), workspace.Number, workspace.Label,
			workspace.TabCount, workspace.PaneCount, workspace.AgentStatus)
		for _, tab := range b.Tabs {
			if tab.WorkspaceID != workspace.WorkspaceID {
				continue
			}
			fmt.Fprintf(&out, "  %s tab %d %s  [%s]\n",
				mark(tab.Focused), tab.Number, tab.Label, tab.AgentStatus)
			for _, agent := range b.Agents {
				if agent.TabID != tab.TabID {
					continue
				}
				fmt.Fprintf(&out, "    %s %-12s %-9s %s\n",
					mark(agent.Focused), agentName(agent), agent.AgentStatus, b.paneLabel(agent.PaneID))
			}
		}
	}
	return out.String()
}

// draw replaces the frame on screen. A failed write is dropped: when the pane
// closes, the terminal can be gone before the shutdown signal arrives, and
// reporting that as a handler error would enter a normal close in Herdr's
// plugin command log as a failure.
func draw(w io.Writer, b board) {
	_, _ = fmt.Fprint(w, clearScreen+render(b))
}

// mark points at the focused entry, which is exclusive across the session.
func mark(focused bool) string {
	if focused {
		return ">"
	}
	return " "
}

// agentName prefers the name the user gave the agent, then the label Herdr
// displays, then the detected agent id.
func agentName(agent herdr.AgentInfo) string {
	return cmp.Or(herdr.Value(agent.Name), herdr.Value(agent.DisplayAgent), herdr.Value(agent.Agent), "agent")
}
