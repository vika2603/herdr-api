package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/plugin"
	"github.com/vika2603/herdr-client/plugin/plugintest"
)

// The registry selects the handler from the environment Herdr injects, so both
// entrypoints can be reached without running the binary. Both handlers start
// by calling the Herdr socket, which is where they stop with no server
// listening; the error naming that socket is what tells dispatch apart from a
// handler that never ran.
func TestDispatchReachesBothEntrypoints(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "herdr-absent.sock")
	tests := []struct {
		name string
		env  *plugin.Env
		kind plugin.EntryKind
	}{
		{
			name: "the board pane",
			env:  plugintest.Env(plugintest.PaneCommand(paneBoard), plugintest.Pane("pane-1")),
			kind: plugin.KindPane,
		},
		{
			name: "the open action",
			env:  plugintest.Env(plugintest.Action(actionOpen), plugintest.Workspace("ws-1")),
			kind: plugin.KindAction,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.env.Kind(); got != test.kind {
				t.Fatalf("Kind() = %q, want %q", got, test.kind)
			}
			test.env.SocketPath = socket
			err := newPlugin().Dispatch(context.Background(), test.env)
			if err == nil {
				t.Fatal("Dispatch() error = nil, want the socket it cannot reach")
			}
			if !strings.Contains(err.Error(), socket) {
				t.Errorf("Dispatch() error = %v, want %s in it", err, socket)
			}
		})
	}
}

// An entrypoint id the binary does not serve is a dispatch error rather than a
// silent success, which is what keeps the manifest and the registry honest.
func TestDispatchRejectsAnUnservedEntrypoint(t *testing.T) {
	err := newPlugin().Dispatch(context.Background(), plugintest.Env(plugintest.PaneCommand("absent")))
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("Dispatch() error = %v, want it to name the unserved pane id", err)
	}
}

// A pane opened as a popup carries no pane id, so the board cannot name its
// own pane and reports no title instead of failing.
func TestNoTitleWithoutAPaneID(t *testing.T) {
	env := plugintest.Env(plugintest.PaneCommand(paneBoard))
	env.SocketPath = filepath.Join(t.TempDir(), "herdr-absent.sock")
	if err := reportTitle(context.Background(), env.Client(), env); err != nil {
		t.Errorf("reportTitle() error = %v, want no call at all", err)
	}
}
