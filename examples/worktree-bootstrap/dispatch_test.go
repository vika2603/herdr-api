package main

import (
	"context"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/plugin/plugintest"
)

// The dispatch tests stop before the first request: every case gives the
// handler something it rejects while deciding, which is what lets them run
// with no Herdr server. What they cover is the wiring, from the environment
// Herdr injects to the handler and the URL it works on.

func TestDispatchThroughTheLinkHandler(t *testing.T) {
	env := plugintest.Env(
		plugintest.Action(actionBootstrap),
		plugintest.LinkHandler("issue"),
		plugintest.ClickedURL("https://example.com/vika2603/herdr-client/issues/7"),
		plugintest.Workspace("ws-1"),
	)

	err := newPlugin().Dispatch(context.Background(), env)
	if err == nil {
		t.Fatal("Dispatch() error = nil, want the clicked URL rejected")
	}
	if !strings.Contains(err.Error(), "example.com") {
		t.Errorf("Dispatch() error = %v, want it to name the clicked URL", err)
	}
}

func TestDispatchOnASelection(t *testing.T) {
	env := plugintest.Env(
		plugintest.Action(actionBootstrap),
		plugintest.SelectedText("release/2.0"),
		plugintest.Workspace("ws-1"),
	)

	err := newPlugin().Dispatch(context.Background(), env)
	if err == nil {
		t.Fatal("Dispatch() error = nil, want the selection rejected")
	}
	if !strings.Contains(err.Error(), "release/2.0") {
		t.Errorf("Dispatch() error = %v, want it to name the selection", err)
	}
}

// A link handler and a selection can both be present, since Herdr sets the
// invocation context either way. The link handler is what invoked the action,
// so its URL wins.
func TestClickedURLPrefersTheLinkHandler(t *testing.T) {
	clicked := plugintest.Env(
		plugintest.LinkHandler("issue"),
		plugintest.ClickedURL(issueURL),
		plugintest.SelectedText("release/2.0"),
	)
	if got := clickedURL(clicked); got != issueURL {
		t.Errorf("clickedURL() = %q, want the clicked URL %q", got, issueURL)
	}

	selected := plugintest.Env(plugintest.SelectedText(issueURL))
	if got := clickedURL(selected); got != issueURL {
		t.Errorf("clickedURL() = %q, want the selection %q", got, issueURL)
	}
}

// Herdr offers the action on a selection in any pane, including one outside a
// workspace, and worktree.create needs the workspace to find the repository.
func TestBootstrapNeedsAWorkspace(t *testing.T) {
	env := plugintest.Env(plugintest.Action(actionBootstrap), plugintest.SelectedText(issueURL))

	err := newPlugin().Dispatch(context.Background(), env)
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Errorf("Dispatch() error = %v, want it to report the missing workspace", err)
	}
}

// An action id the binary does not serve is a dispatch error rather than a
// silent success, which is what ties herdr-plugin.toml to the registry.
func TestDispatchRejectsAnUnregisteredAction(t *testing.T) {
	err := newPlugin().Dispatch(context.Background(), plugintest.Env(plugintest.Action("open")))
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Errorf("Dispatch() error = %v, want no handler for %q", err, "open")
	}
}
