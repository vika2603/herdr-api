package main

import (
	"slices"
	"testing"

	"github.com/vika2603/herdr-api/plugin/manifest"
)

// TestManifest checks herdr-plugin.toml against the rules Herdr enforces and
// against the entrypoints this binary serves.
func TestManifest(t *testing.T) {
	parsed, warnings, err := manifest.Parse("herdr-plugin.toml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}

	if parsed.ID != "example.agent-status" {
		t.Errorf("id = %q", parsed.ID)
	}
	if len(parsed.Build) != 1 || len(parsed.Startup) != 1 {
		t.Errorf("build = %v, startup = %v, want one entry each", parsed.Build, parsed.Startup)
	}

	if len(parsed.Actions) != 1 {
		t.Fatalf("actions = %v, want exactly the one this binary serves", parsed.Actions)
	}
	if got := parsed.Actions[0].ID; got != actionShow {
		t.Errorf("action id = %q, want %q", got, actionShow)
	}

	if len(parsed.Events) != 1 {
		t.Fatalf("events = %v, want exactly the one this binary serves", parsed.Events)
	}
	if got := parsed.Events[0].On; got != hookEvent {
		t.Errorf("event = %q, want %q", got, hookEvent)
	}

	// Every entrypoint runs the binary the build step produces, which is what
	// lets plugin.Run dispatch them.
	if want := []string{"go", "build", "-o", "herdr-agent-status", "."}; !slices.Equal(parsed.Build[0].Command, want) {
		t.Errorf("build command = %v, want %v", parsed.Build[0].Command, want)
	}
	for _, command := range [][]string{
		parsed.Startup[0].Command,
		parsed.Actions[0].Command,
		parsed.Events[0].Command,
	} {
		if want := []string{"./herdr-agent-status"}; !slices.Equal(command, want) {
			t.Errorf("command = %v, want %v", command, want)
		}
	}
}
