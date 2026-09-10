package manifest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// idMaxChars is the length limit herdr applies to plugin ids and to the local
// ids of actions, panes and link handlers.
const idMaxChars = 120

// Error is a manifest validation failure. Code carries the error code herdr
// v0.9.0 reports for the same rule; the codes for values herdr rejects while
// deserializing, namely unknown action contexts and pane placements, follow
// the same naming.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func newError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Validate checks every rule herdr applies when it loads a manifest and
// returns the first violation. Warnings are returned separately, as they are
// in herdr: an event hook naming an event outside the set herdr fires hooks
// for, and a manifest that leaves platforms undeclared.
//
// Values are validated with surrounding whitespace trimmed, the way herdr
// normalizes them, but Validate does not rewrite the manifest. The one rule
// that cannot be checked here is herdr's refusal to load a plugin whose
// min_herdr_version is newer than the running binary; this package validates
// the version's syntax only.
func (m *Manifest) Validate() ([]string, error) {
	if !validPluginID(m.ID) {
		return nil, newError("invalid_plugin_id",
			"plugin id %q must be 1 to %d characters of ASCII letters, digits, ':', '.', '_' or '-'", m.ID, idMaxChars)
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, newError("invalid_plugin_name", "plugin name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, newError("invalid_plugin_version", "plugin version is required")
	}
	if err := validateMinHerdrVersion(m.MinHerdrVersion); err != nil {
		return nil, err
	}
	if err := validatePlatforms(m.Platforms, "manifest"); err != nil {
		return nil, err
	}
	for i, build := range m.Build {
		where := fmt.Sprintf("build[%d]", i)
		if err := validatePlatforms(build.Platforms, where); err != nil {
			return nil, err
		}
		if err := validateCommand(build.Command, where); err != nil {
			return nil, err
		}
	}
	for i, startup := range m.Startup {
		where := fmt.Sprintf("startup[%d]", i)
		if err := validatePlatforms(startup.Platforms, where); err != nil {
			return nil, err
		}
		if err := validateCommand(startup.Command, where); err != nil {
			return nil, err
		}
	}
	if err := m.validateActions(); err != nil {
		return nil, err
	}
	if err := m.validateEvents(); err != nil {
		return nil, err
	}
	if err := m.validatePanes(); err != nil {
		return nil, err
	}
	if err := m.validateLinkHandlers(); err != nil {
		return nil, err
	}

	var warnings []string
	for _, event := range m.Events {
		if name := strings.TrimSpace(event.On); !isHookEventName(name) {
			warnings = append(warnings, fmt.Sprintf("unknown event '%s'", name))
		}
	}
	if m.Platforms == nil {
		warnings = append(warnings, "manifest does not declare platforms; platform support unknown")
	}
	return warnings, nil
}

func (m *Manifest) validateActions() error {
	seen := make(map[string]struct{}, len(m.Actions))
	for i, action := range m.Actions {
		where := fmt.Sprintf("actions[%d]", i)
		id := strings.TrimSpace(action.ID)
		if !validLocalID(id) {
			return newError("invalid_plugin_action_id",
				"%s: action id %q must be 1 to %d characters of ASCII letters, digits, ':', '_' or '-'", where, action.ID, idMaxChars)
		}
		if strings.TrimSpace(action.Title) == "" {
			return newError("invalid_plugin_action_title", "%s: action title is required", where)
		}
		for _, context := range action.Contexts {
			switch context {
			case ContextGlobal, ContextWorkspace, ContextTab, ContextPane, ContextSelection:
			default:
				return newError("invalid_plugin_action_context", "%s: unknown action context %q", where, context)
			}
		}
		if err := validatePlatforms(action.Platforms, where); err != nil {
			return err
		}
		if err := validateCommand(action.Command, where); err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return newError("duplicate_plugin_action_id", "duplicate action id '%s'", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (m *Manifest) validateEvents() error {
	for i, event := range m.Events {
		where := fmt.Sprintf("events[%d]", i)
		if strings.TrimSpace(event.On) == "" {
			return newError("invalid_plugin_event", "%s: event name is required", where)
		}
		if err := validatePlatforms(event.Platforms, where); err != nil {
			return err
		}
		if err := validateCommand(event.Command, where); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) validatePanes() error {
	seen := make(map[string]struct{}, len(m.Panes))
	for i, pane := range m.Panes {
		where := fmt.Sprintf("panes[%d]", i)
		id := strings.TrimSpace(pane.ID)
		if !validLocalID(id) {
			return newError("invalid_plugin_pane_id",
				"%s: pane id %q must be 1 to %d characters of ASCII letters, digits, ':', '_' or '-'", where, pane.ID, idMaxChars)
		}
		if strings.TrimSpace(pane.Title) == "" {
			return newError("invalid_plugin_pane_title", "%s: pane title is required", where)
		}
		switch pane.Placement {
		case "", PlacementOverlay, PlacementPopup, PlacementSplit, PlacementTab, PlacementZoomed:
		default:
			return newError("invalid_plugin_pane_placement", "%s: unknown pane placement %q", where, pane.Placement)
		}
		if err := validatePlatforms(pane.Platforms, where); err != nil {
			return err
		}
		if err := validateCommand(pane.Command, where); err != nil {
			return err
		}
		if pane.Placement != PlacementPopup && (pane.Width != nil || pane.Height != nil) {
			return newError("invalid_plugin_pane_size",
				"%s: pane width and height are only supported when placement is popup", where)
		}
		if _, duplicate := seen[id]; duplicate {
			return newError("duplicate_plugin_pane_id", "duplicate pane id '%s'", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (m *Manifest) validateLinkHandlers() error {
	seen := make(map[string]struct{}, len(m.LinkHandlers))
	for i, handler := range m.LinkHandlers {
		where := fmt.Sprintf("link_handlers[%d]", i)
		id := strings.TrimSpace(handler.ID)
		if !validLocalID(id) {
			return newError("invalid_plugin_link_handler_id",
				"%s: link handler id %q must be 1 to %d characters of ASCII letters, digits, ':', '_' or '-'", where, handler.ID, idMaxChars)
		}
		if strings.TrimSpace(handler.Title) == "" {
			return newError("invalid_plugin_link_handler_title", "%s: link handler title is required", where)
		}
		pattern := strings.TrimSpace(handler.Pattern)
		if pattern == "" {
			return newError("invalid_plugin_link_handler_pattern", "%s: link handler pattern is required", where)
		}
		// Herdr matches the pattern with the Rust regex crate. Go's regexp
		// accepts a very similar syntax, so this catches malformed patterns
		// without guaranteeing byte-identical acceptance.
		if _, err := regexp.Compile(pattern); err != nil {
			return newError("invalid_plugin_link_handler_pattern", "%s: %s", where, err)
		}
		action := strings.TrimSpace(handler.Action)
		if !validLocalID(action) {
			return newError("invalid_plugin_link_handler_action",
				"%s: link handler action %q is not a valid action id", where, handler.Action)
		}
		if !m.hasAction(action) {
			return newError("invalid_plugin_link_handler_action",
				"link handler '%s' references unknown action '%s'", id, action)
		}
		if err := validatePlatforms(handler.Platforms, where); err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return newError("duplicate_plugin_link_handler_id", "duplicate link handler id '%s'", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (m *Manifest) hasAction(id string) bool {
	for _, action := range m.Actions {
		if strings.TrimSpace(action.ID) == id {
			return true
		}
	}
	return false
}

// validateMinHerdrVersion accepts the versions herdr's Version::parse accepts:
// three dot-separated numbers with an optional "v" prefix.
func validateMinHerdrVersion(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return newError("invalid_plugin_min_herdr_version", "plugin min_herdr_version is required")
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != 3 {
		return newError("invalid_plugin_min_herdr_version",
			"plugin min_herdr_version %q must be a semantic version like 0.9.0", value)
	}
	for _, part := range parts {
		if _, err := strconv.ParseUint(part, 10, 32); err != nil {
			return newError("invalid_plugin_min_herdr_version",
				"plugin min_herdr_version %q must be a semantic version like 0.9.0", value)
		}
	}
	return nil
}

// validatePlatforms rejects an empty array and unknown values. An absent
// array, which reaches here as nil, is reported as a warning instead.
func validatePlatforms(platforms []Platform, where string) error {
	if platforms == nil {
		return nil
	}
	if len(platforms) == 0 {
		return newError("invalid_plugin_platform",
			"%s: platforms must not be an empty array; omit the field to leave platforms undeclared", where)
	}
	for _, platform := range platforms {
		switch platform {
		case PlatformLinux, PlatformMacOS, PlatformWindows:
		default:
			return newError("invalid_plugin_platform", "%s: unknown platform %q", where, platform)
		}
	}
	return nil
}

// validateCommand requires a non-empty argv of non-empty arguments. Herdr runs
// it without a shell.
func validateCommand(command []string, where string) error {
	if len(command) == 0 {
		return newError("invalid_plugin_command", "%s: command must contain non-empty argv strings", where)
	}
	for _, arg := range command {
		if arg == "" {
			return newError("invalid_plugin_command", "%s: command must contain non-empty argv strings", where)
		}
	}
	return nil
}

// validPluginID reports whether value is a plugin id: ASCII letters, digits,
// ':', '.', '_' and '-'.
func validPluginID(value string) bool {
	return validIdentifier(value, true)
}

// validLocalID reports whether value is an id local to a plugin, which unlike
// a plugin id may not contain '.' because Herdr joins the two with a dot.
func validLocalID(value string) bool {
	return validIdentifier(value, false)
}

func validIdentifier(value string, allowDot bool) bool {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > idMaxChars {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == ':' || c == '_' || c == '-':
		case c == '.' && allowDot:
		default:
			return false
		}
	}
	return true
}
