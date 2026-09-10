package herdr

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	envSocketPath = "HERDR_SOCKET_PATH"
	envSession    = "HERDR_SESSION"

	// defaultSessionName names the session that lives directly in the config
	// directory rather than under sessions/.
	defaultSessionName = "default"

	appDirName     = "herdr"
	socketFileName = "herdr.sock"
	sessionsDir    = "sessions"

	maxSessionNameLen = 64
)

// ResolveSocketPath returns the API socket path of a Herdr session.
//
// A non-empty session selects that session's socket,
// <config>/sessions/<session>/herdr.sock, and takes precedence over the
// environment. The session name "default" refers to the socket of the
// default session, <config>/herdr.sock.
//
// With an empty session the path is taken from HERDR_SOCKET_PATH, then from
// the session named by HERDR_SESSION, and finally from the default session.
//
// <config> is $XDG_CONFIG_HOME/herdr when that variable is set, and the
// platform config directory otherwise.
//
// The last path segment is always herdr. A herdr built with debug assertions
// uses herdr-dev instead, so a resolved path only reaches a release build.
// Set HERDR_SOCKET_PATH to reach a debug-built server.
func ResolveSocketPath(session string) (string, error) {
	if session != "" {
		name, err := normalizeSessionName(session)
		if err != nil {
			return "", err
		}
		return sessionSocketPath(name), nil
	}
	if path := os.Getenv(envSocketPath); path != "" {
		return path, nil
	}
	if session := os.Getenv(envSession); session != "" {
		name, err := normalizeSessionName(session)
		if err != nil {
			return "", fmt.Errorf("herdr: %s: %w", envSession, err)
		}
		return sessionSocketPath(name), nil
	}
	return sessionSocketPath(""), nil
}

// sessionSocketPath returns the socket path of the named session; an empty
// name selects the default session.
func sessionSocketPath(name string) string {
	dir := configDir()
	if name == "" {
		return filepath.Join(dir, socketFileName)
	}
	return filepath.Join(dir, sessionsDir, name, socketFileName)
}

func configDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, appDirName)
	}
	return platformConfigDir()
}

// normalizeSessionName validates a session name the way herdr does and maps
// the default session to the empty name. Names are rejected rather than
// ignored because they become a path segment.
func normalizeSessionName(name string) (string, error) {
	if name == defaultSessionName {
		return "", nil
	}
	if len(name) > maxSessionNameLen {
		return "", fmt.Errorf("session name %q is longer than %d bytes", name, maxSessionNameLen)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("session name %q is not allowed", name)
	}
	for i := 0; i < len(name); i++ {
		if !isSessionNameByte(name[i]) {
			return "", fmt.Errorf("session name %q may only contain ASCII letters, digits, '.', '_' and '-'", name)
		}
	}
	return name, nil
}

func isSessionNameByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '.' || b == '_' || b == '-':
		return true
	default:
		return false
	}
}
