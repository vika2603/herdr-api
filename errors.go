package herdr

import "errors"

// Error codes reported by the server in an error response. Comparing a
// Code against a plain string stays valid; these constants only save the
// caller from repeating the spelling.
const (
	ErrCodeInvalidRequest              = "invalid_request"
	ErrCodeInvalidParams               = "invalid_params"
	ErrCodeInternalError               = "internal_error"
	ErrCodeTimeout                     = "timeout"
	ErrCodeNotFound                    = "not_found"
	ErrCodePaneNotFound                = "pane_not_found"
	ErrCodeWorkspaceNotFound           = "workspace_not_found"
	ErrCodeTabNotFound                 = "tab_not_found"
	ErrCodeAgentNotFound               = "agent_not_found"
	ErrCodePluginNotFound              = "plugin_not_found"
	ErrCodePluginDisabled              = "plugin_disabled"
	ErrCodePluginPaneNotFound          = "plugin_pane_not_found"
	ErrCodePlatformUnsupported         = "platform_unsupported"
	ErrCodeFeatureDisabled             = "feature_disabled"
	ErrCodeUIBusy                      = "ui_busy"
	ErrCodePopupNotOpen                = "popup_not_open"
	ErrCodeStreamConflict              = "stream_conflict"
	ErrCodeStreamClosed                = "stream_closed"
	ErrCodeAgentBlocked                = "agent_blocked"
	ErrCodeAgentPromptStalled          = "agent_prompt_stalled"
	ErrCodeWorkspaceGroupCloseRequired = "workspace_group_close_required"
	ErrCodeUnsupportedInAppMode        = "unsupported_in_app_mode"
)

// Error is an error response from the server. Method is the method that was
// called; it is empty when the error did not originate from a call.
type Error struct {
	Method  string
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Method == "" {
		return e.Code + ": " + e.Message
	}
	return e.Method + ": " + e.Code + ": " + e.Message
}

// IsCode reports whether err is, or wraps, a server *Error carrying code.
func IsCode(err error, code string) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
