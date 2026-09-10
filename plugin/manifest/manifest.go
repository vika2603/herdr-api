// Package manifest parses and validates herdr-plugin.toml files.
//
// The structs mirror the manifest herdr v0.9.0 reads in
// src/app/api/plugins/manifest.rs. Unknown keys are ignored, as they are by
// herdr; every other rule herdr enforces is reported by Validate.
package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
)

// Platform is a value of a manifest platforms array.
type Platform string

// Platforms a plugin can declare support for.
const (
	PlatformLinux   Platform = "linux"
	PlatformMacOS   Platform = "macos"
	PlatformWindows Platform = "windows"
)

// ActionContext is a value of an action's contexts array. It tells Herdr where
// the action is offered.
type ActionContext string

// Contexts an action can be offered in.
const (
	ContextGlobal    ActionContext = "global"
	ContextWorkspace ActionContext = "workspace"
	ContextTab       ActionContext = "tab"
	ContextPane      ActionContext = "pane"
	ContextSelection ActionContext = "selection"
)

// Placement is how Herdr opens a plugin pane.
type Placement string

// Placements a pane entrypoint can declare. An empty Placement means
// PlacementOverlay, the default Herdr applies.
const (
	PlacementOverlay Placement = "overlay"
	PlacementPopup   Placement = "popup"
	PlacementSplit   Placement = "split"
	PlacementTab     Placement = "tab"
	PlacementZoomed  Placement = "zoomed"
)

// Manifest is a herdr-plugin.toml file.
//
// A nil Platforms slice means the manifest left platforms undeclared, which
// Validate reports as a warning; an empty non-nil slice is an error.
type Manifest struct {
	ID              string        `toml:"id"`
	Name            string        `toml:"name"`
	Version         string        `toml:"version"`
	MinHerdrVersion string        `toml:"min_herdr_version"`
	Description     string        `toml:"description"`
	Platforms       []Platform    `toml:"platforms"`
	Build           []Build       `toml:"build"`
	Startup         []Startup     `toml:"startup"`
	Actions         []Action      `toml:"actions"`
	Events          []EventHook   `toml:"events"`
	Panes           []Pane        `toml:"panes"`
	LinkHandlers    []LinkHandler `toml:"link_handlers"`
}

// Build is a [[build]] entry, run once at install time.
type Build struct {
	Command   []string   `toml:"command"`
	Platforms []Platform `toml:"platforms"`
}

// Startup is a [[startup]] entry, run once per session for an enabled plugin.
type Startup struct {
	Command   []string   `toml:"command"`
	Platforms []Platform `toml:"platforms"`
}

// Action is an [[actions]] entry, a command a user or keybinding can invoke.
type Action struct {
	ID          string          `toml:"id"`
	Title       string          `toml:"title"`
	Description string          `toml:"description"`
	Contexts    []ActionContext `toml:"contexts"`
	Command     []string        `toml:"command"`
	Platforms   []Platform      `toml:"platforms"`
}

// EventHook is an [[events]] entry. On is a dotted event name such as
// "worktree.created".
type EventHook struct {
	On        string     `toml:"on"`
	Command   []string   `toml:"command"`
	Platforms []Platform `toml:"platforms"`
}

// Pane is a [[panes]] entry, a command Herdr runs in a plugin-owned pane.
// Width and Height apply to popup placement only.
type Pane struct {
	ID          string     `toml:"id"`
	Title       string     `toml:"title"`
	Description string     `toml:"description"`
	Placement   Placement  `toml:"placement"`
	Width       *PopupSize `toml:"width"`
	Height      *PopupSize `toml:"height"`
	Command     []string   `toml:"command"`
	Platforms   []Platform `toml:"platforms"`
}

// LinkHandler is a [[link_handlers]] entry. Pattern is a regular expression
// matched against a clicked terminal URL and Action names an action of the
// same plugin.
type LinkHandler struct {
	ID        string     `toml:"id"`
	Title     string     `toml:"title"`
	Pattern   string     `toml:"pattern"`
	Action    string     `toml:"action"`
	Platforms []Platform `toml:"platforms"`
}

// Decode reads one manifest from r without validating it. A field the file
// omits keeps its zero value, so missing required fields surface from
// Validate rather than here.
func Decode(r io.Reader) (*Manifest, error) {
	var m Manifest
	if _, err := toml.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &m, nil
}

// Parse reads and validates the manifest file at path. It returns the
// validation warnings only when validation succeeds; on any error the manifest
// is nil.
func Parse(path string) (*Manifest, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	m, err := Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	warnings, err := m.Validate()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, warnings, nil
}
