package herdr

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setConfigEnv points the resolution at a known config directory and clears
// the two variables the resolution consults.
func setConfigEnv(t *testing.T, configHome string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HERDR_SESSION", "")
}

func TestResolveSocketPathExplicitSessionWinsOverEnvironment(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SOCKET_PATH", "/env/socket.sock")
	t.Setenv("HERDR_SESSION", "other")

	got, err := ResolveSocketPath("work")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/cfg", "herdr", "sessions", "work", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathExplicitDefaultSession(t *testing.T) {
	setConfigEnv(t, "/cfg")

	got, err := ResolveSocketPath("default")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/cfg", "herdr", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathEnvironmentOrder(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SOCKET_PATH", "/env/socket.sock")
	t.Setenv("HERDR_SESSION", "work")

	got, err := ResolveSocketPath("")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	if got != "/env/socket.sock" {
		t.Errorf("got %q, want the HERDR_SOCKET_PATH value", got)
	}
}

func TestResolveSocketPathSessionFromEnvironment(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SESSION", "work")

	got, err := ResolveSocketPath("")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/cfg", "herdr", "sessions", "work", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathDefaultSessionFromEnvironment(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SESSION", "default")

	got, err := ResolveSocketPath("")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/cfg", "herdr", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathDefault(t *testing.T) {
	setConfigEnv(t, "/cfg")

	got, err := ResolveSocketPath("")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/cfg", "herdr", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathHomeFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the config directory follows APPDATA on Windows")
	}
	setConfigEnv(t, "")
	t.Setenv("HOME", "/home/tester")

	got, err := ResolveSocketPath("")
	if err != nil {
		t.Fatalf("ResolveSocketPath: %v", err)
	}
	want := filepath.Join("/home/tester", ".config", "herdr", "herdr.sock")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketPathRejectsInvalidSessionNames(t *testing.T) {
	setConfigEnv(t, "/cfg")

	for _, name := range []string{"..", ".", "bad/name", "../escape", "with space", strings.Repeat("a", 65)} {
		if got, err := ResolveSocketPath(name); err == nil {
			t.Errorf("ResolveSocketPath(%q) = %q, want an error", name, got)
		}
	}
}

func TestResolveSocketPathRejectsInvalidSessionFromEnvironment(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SESSION", "bad/name")

	if got, err := ResolveSocketPath(""); err == nil {
		t.Errorf("ResolveSocketPath(\"\") = %q, want an error", got)
	}
}

func TestNewFromEnvUsesResolvedPath(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SOCKET_PATH", "/env/socket.sock")

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if got := client.SocketPath(); got != "/env/socket.sock" {
		t.Errorf("SocketPath() = %q", got)
	}
}

func TestNewFromEnvReportsInvalidSession(t *testing.T) {
	setConfigEnv(t, "/cfg")
	t.Setenv("HERDR_SESSION", "bad/name")

	if _, err := NewFromEnv(); err == nil {
		t.Error("NewFromEnv() = nil error, want an error")
	}
}
