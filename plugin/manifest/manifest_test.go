package manifest

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		wantWarnings []string
		check        func(t *testing.T, m *Manifest)
	}{
		{
			name: "documentation example",
			file: "layout.toml",
			check: func(t *testing.T, m *Manifest) {
				want := &Manifest{
					ID:              "example.layout",
					Name:            "Layout",
					Version:         "0.1.0",
					MinHerdrVersion: "0.7.0",
					Description:     "Apply project layouts",
					Platforms:       []Platform{PlatformLinux, PlatformMacOS, PlatformWindows},
					Build: []Build{
						{Command: []string{"npm", "ci"}},
						{Command: []string{"npm", "run", "build"}, Platforms: []Platform{PlatformLinux, PlatformMacOS}},
					},
					Startup: []Startup{{Command: []string{"node", "dist/restore.js"}}},
					Actions: []Action{{
						ID:       "apply",
						Title:    "Apply layout",
						Contexts: []ActionContext{ContextWorkspace},
						Command:  []string{"node", "dist/apply.js"},
					}},
					Events: []EventHook{{On: "worktree.created", Command: []string{"herdr", "workspace", "list"}}},
					Panes: []Pane{{
						ID:        "board",
						Title:     "Project board",
						Placement: PlacementOverlay,
						Command:   []string{"herdr-board"},
					}},
					LinkHandlers: []LinkHandler{{
						ID:      "github-issue",
						Title:   "Open GitHub issue",
						Pattern: `^https://github\.com/[^/]+/[^/]+/(issues|pull)/[0-9]+$`,
						Action:  "apply",
					}},
				}
				if !reflect.DeepEqual(m, want) {
					t.Errorf("manifest =\n%+v\nwant\n%+v", m, want)
				}
			},
		},
		{
			name: "first plugin example",
			file: "workspace-tools.toml",
			check: func(t *testing.T, m *Manifest) {
				if m.ID != "example.workspace-tools" || len(m.Actions) != 1 {
					t.Fatalf("manifest = %+v", m)
				}
				if m.Actions[0].ID != "list-workspaces" {
					t.Errorf("action id = %q", m.Actions[0].ID)
				}
			},
		},
		{
			name: "popup pane example",
			file: "picker.toml",
			check: func(t *testing.T, m *Manifest) {
				pane := m.Panes[0]
				if pane.Placement != PlacementPopup {
					t.Errorf("placement = %q, want %q", pane.Placement, PlacementPopup)
				}
				if want := (PopupSize{Percent: 80}); *pane.Width != want {
					t.Errorf("width = %+v, want %+v", *pane.Width, want)
				}
				if want := (PopupSize{Cells: 20}); *pane.Height != want {
					t.Errorf("height = %+v, want %+v", *pane.Height, want)
				}
			},
		},
		{
			name: "installed auto-title plugin",
			file: "auto-title.toml",
			check: func(t *testing.T, m *Manifest) {
				if m.ID != "herdr.auto-title" || m.MinHerdrVersion != "0.8.2" {
					t.Fatalf("manifest = %+v", m)
				}
				if len(m.Build) != 2 || len(m.Startup) != 2 {
					t.Fatalf("build = %+v, startup = %+v", m.Build, m.Startup)
				}
				if got := m.Startup[1].Platforms; !reflect.DeepEqual(got, []Platform{PlatformWindows}) {
					t.Errorf("startup[1].Platforms = %+v", got)
				}
			},
		},
		{
			name:         "undeclared platforms and unknown event warn",
			file:         "no-platforms.toml",
			wantWarnings: []string{"unknown event 'pane.exploded'", "manifest does not declare platforms; platform support unknown"},
			check: func(t *testing.T, m *Manifest) {
				if m.Platforms != nil {
					t.Errorf("Platforms = %+v, want nil", m.Platforms)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, warnings, err := Parse(filepath.Join("testdata", "valid", tt.file))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(warnings, tt.wantWarnings) {
				t.Errorf("warnings = %#v, want %#v", warnings, tt.wantWarnings)
			}
			tt.check(t, m)
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantCode string
	}{
		{name: "id missing", file: "id-missing.toml", wantCode: "invalid_plugin_id"},
		{name: "id blank", file: "id-blank.toml", wantCode: "invalid_plugin_id"},
		{name: "id with a rejected character", file: "id-bad-character.toml", wantCode: "invalid_plugin_id"},
		{name: "id longer than the limit", file: "id-too-long.toml", wantCode: "invalid_plugin_id"},
		{name: "name missing", file: "name-missing.toml", wantCode: "invalid_plugin_name"},
		{name: "version missing", file: "version-missing.toml", wantCode: "invalid_plugin_version"},
		{name: "min_herdr_version missing", file: "min-herdr-version-missing.toml", wantCode: "invalid_plugin_min_herdr_version"},
		{name: "min_herdr_version not a semantic version", file: "min-herdr-version-not-semver.toml", wantCode: "invalid_plugin_min_herdr_version"},
		{name: "platforms empty", file: "platforms-empty.toml", wantCode: "invalid_plugin_platform"},
		{name: "platforms unknown value", file: "platforms-unknown.toml", wantCode: "invalid_plugin_platform"},
		{name: "build command empty", file: "build-command-empty.toml", wantCode: "invalid_plugin_command"},
		{name: "build command with a blank argument", file: "build-command-blank-argument.toml", wantCode: "invalid_plugin_command"},
		{name: "build platforms empty", file: "build-platforms-empty.toml", wantCode: "invalid_plugin_platform"},
		{name: "startup command missing", file: "startup-command-missing.toml", wantCode: "invalid_plugin_command"},
		{name: "action id missing", file: "action-id-missing.toml", wantCode: "invalid_plugin_action_id"},
		{name: "action id with a dot", file: "action-id-dotted.toml", wantCode: "invalid_plugin_action_id"},
		{name: "action title missing", file: "action-title-missing.toml", wantCode: "invalid_plugin_action_title"},
		{name: "action context unknown", file: "action-context-unknown.toml", wantCode: "invalid_plugin_action_context"},
		{name: "action command empty", file: "action-command-empty.toml", wantCode: "invalid_plugin_command"},
		{name: "duplicate action ids", file: "action-id-duplicate.toml", wantCode: "duplicate_plugin_action_id"},
		{name: "event name missing", file: "event-on-missing.toml", wantCode: "invalid_plugin_event"},
		{name: "event command empty", file: "event-command-empty.toml", wantCode: "invalid_plugin_command"},
		{name: "pane id with a dot", file: "pane-id-dotted.toml", wantCode: "invalid_plugin_pane_id"},
		{name: "pane title missing", file: "pane-title-missing.toml", wantCode: "invalid_plugin_pane_title"},
		{name: "pane placement unknown", file: "pane-placement-unknown.toml", wantCode: "invalid_plugin_pane_placement"},
		{name: "pane size without popup placement", file: "pane-size-without-popup.toml", wantCode: "invalid_plugin_pane_size"},
		{name: "pane command empty", file: "pane-command-empty.toml", wantCode: "invalid_plugin_command"},
		{name: "duplicate pane ids", file: "pane-id-duplicate.toml", wantCode: "duplicate_plugin_pane_id"},
		{name: "link handler id with a dot", file: "link-handler-id-dotted.toml", wantCode: "invalid_plugin_link_handler_id"},
		{name: "link handler title missing", file: "link-handler-title-missing.toml", wantCode: "invalid_plugin_link_handler_title"},
		{name: "link handler pattern missing", file: "link-handler-pattern-missing.toml", wantCode: "invalid_plugin_link_handler_pattern"},
		{name: "link handler pattern not a regular expression", file: "link-handler-pattern-invalid.toml", wantCode: "invalid_plugin_link_handler_pattern"},
		{name: "link handler action unknown", file: "link-handler-action-unknown.toml", wantCode: "invalid_plugin_link_handler_action"},
		{name: "link handler action not a local id", file: "link-handler-action-dotted.toml", wantCode: "invalid_plugin_link_handler_action"},
		{name: "duplicate link handler ids", file: "link-handler-id-duplicate.toml", wantCode: "duplicate_plugin_link_handler_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", "invalid", tt.file)
			m, warnings, err := Parse(path)
			if err == nil {
				t.Fatalf("Parse() succeeded, want code %q", tt.wantCode)
			}
			if m != nil || warnings != nil {
				t.Errorf("manifest = %+v, warnings = %+v, want nil", m, warnings)
			}
			var verr *Error
			if !errors.As(err, &verr) {
				t.Fatalf("error = %v, want *manifest.Error", err)
			}
			if verr.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", verr.Code, tt.wantCode)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestParseDecodeErrors(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		wantMessage string
	}{
		{name: "malformed toml", file: "malformed.toml", wantMessage: "decode manifest"},
		{name: "string size without a percent sign", file: "size-string-not-percent.toml", wantMessage: `popup size "20" must be a percentage`},
		{name: "percentage out of range", file: "size-percent-out-of-range.toml", wantMessage: `popup size "0%" must be a percentage between`},
		{name: "cell count out of range", file: "size-cells-out-of-range.toml", wantMessage: "popup size 70000 is out of range"},
		{name: "size of the wrong type", file: "size-wrong-type.toml", wantMessage: "popup size must be an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse(filepath.Join("testdata", "invalid", tt.file))
			if err == nil {
				t.Fatalf("Parse() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMessage)
			}
		})
	}
}

func TestParseMissingFile(t *testing.T) {
	if _, _, err := Parse(filepath.Join("testdata", "valid", "absent.toml")); err == nil {
		t.Fatal("Parse() succeeded for a missing file")
	}
}

func TestDecodeDoesNotValidate(t *testing.T) {
	m, err := Decode(strings.NewReader("name = \"Plugin\"\nunknown_key = 1\n"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if m.ID != "" || m.Name != "Plugin" {
		t.Errorf("manifest = %+v", m)
	}
	if _, err := m.Validate(); err == nil {
		t.Error("Validate() succeeded for a manifest without an id")
	}
}

func TestValidatePaneSizeDefaultPlacement(t *testing.T) {
	tests := []struct {
		name      string
		placement Placement
		wantErr   bool
	}{
		{name: "popup accepts a size", placement: PlacementPopup},
		{name: "default placement rejects a size", placement: "", wantErr: true},
		{name: "split rejects a size", placement: PlacementSplit, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				ID:              "example.plugin",
				Name:            "Plugin",
				Version:         "0.1.0",
				MinHerdrVersion: "0.9.0",
				Platforms:       []Platform{PlatformLinux},
				Panes: []Pane{{
					ID:        "picker",
					Title:     "Picker",
					Placement: tt.placement,
					Height:    &PopupSize{Cells: 20},
					Command:   []string{"true"},
				}},
			}
			_, err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				var verr *Error
				if !errors.As(err, &verr) || verr.Code != "invalid_plugin_pane_size" {
					t.Errorf("error = %v, want invalid_plugin_pane_size", err)
				}
			}
		})
	}
}

func TestValidateAcceptsSurroundingWhitespace(t *testing.T) {
	m := &Manifest{
		ID:              " example.plugin ",
		Name:            " Plugin ",
		Version:         " 0.1.0 ",
		MinHerdrVersion: " 0.9.0 ",
		Platforms:       []Platform{PlatformLinux},
		Actions:         []Action{{ID: " apply ", Title: " Apply ", Command: []string{"true"}}},
		LinkHandlers:    []LinkHandler{{ID: " open ", Title: " Open ", Pattern: " ^https:// ", Action: " apply "}},
	}
	warnings, err := m.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if warnings != nil {
		t.Errorf("warnings = %#v, want none", warnings)
	}
	if m.ID != " example.plugin " {
		t.Errorf("Validate() rewrote ID to %q", m.ID)
	}
}
