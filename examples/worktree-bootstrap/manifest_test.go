package main

import (
	"testing"

	"github.com/vika2603/herdr-client/plugin/manifest"
	"github.com/vika2603/herdr-client/plugin/plugintest"
)

const manifestPath = "herdr-plugin.toml"

// TestManifest checks herdr-plugin.toml against the rules Herdr enforces and
// against the entrypoints this binary serves.
func TestManifest(t *testing.T) {
	plugintest.CheckManifest(t, manifestPath, newPlugin())
}

// TestLinkPatternMatchesTheManifest ties the pattern Herdr matches to the one
// this binary parses. They are two copies of one decision: a URL Herdr offers
// the handler for that main.go cannot parse would fail only on a click.
func TestLinkPatternMatchesTheManifest(t *testing.T) {
	parsed, _, err := manifest.Parse(manifestPath)
	if err != nil {
		t.Fatalf("%s: %v", manifestPath, err)
	}
	if len(parsed.LinkHandlers) != 1 {
		t.Fatalf("%s declares %d link handlers, want 1", manifestPath, len(parsed.LinkHandlers))
	}
	handler := parsed.LinkHandlers[0]
	if handler.Pattern != linkPattern {
		t.Errorf("manifest pattern = %q, main.go pattern = %q", handler.Pattern, linkPattern)
	}
	if handler.Action != actionBootstrap {
		t.Errorf("link handler action = %q, want %q", handler.Action, actionBootstrap)
	}
}

func TestLinkPatternSelectsIssueURLs(t *testing.T) {
	matching := []string{
		"https://github.com/vika2603/herdr-client/issues/7",
		"https://github.com/vika2603/herdr-client/pull/42",
		// A link clicked in a pane carries whatever the text held around
		// the number, so a path or a fragment after it still matches.
		"https://github.com/vika2603/herdr-client/pull/42/files",
		"https://github.com/vika2603/herdr-client/issues/7#issuecomment-1",
	}
	notMatching := []string{
		"https://github.com/vika2603/herdr-client",
		"https://github.com/vika2603/herdr-client/issues",
		"https://github.com/vika2603/herdr-client/issues/none",
		// "12abc" is not issue 12, and "pulls" is not the pull request path.
		"https://github.com/vika2603/herdr-client/issues/12abc",
		"https://github.com/vika2603/herdr-client/pulls/42",
		"https://example.com/vika2603/herdr-client/issues/7",
		// Herdr matches the pattern unanchored at the end only.
		"http://github.com/vika2603/herdr-client/issues/7",
		"see https://github.com/vika2603/herdr-client/issues/7",
	}

	for _, url := range matching {
		if !issueLink.MatchString(url) {
			t.Errorf("%q does not match, want a match", url)
		}
	}
	for _, url := range notMatching {
		if issueLink.MatchString(url) {
			t.Errorf("%q matches, want no match", url)
		}
	}
}
