package main

import (
	"testing"

	"github.com/vika2603/herdr-client/plugin/plugintest"
)

// TestManifest checks herdr-plugin.toml against the rules Herdr enforces and
// against the entrypoints this binary serves.
func TestManifest(t *testing.T) {
	plugintest.CheckManifest(t, "herdr-plugin.toml", newPlugin())
}
