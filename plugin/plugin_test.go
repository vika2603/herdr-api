package plugin

import (
	"errors"
	"reflect"
	"testing"
)

// lookupFrom turns a map into an os.LookupEnv-compatible function.
func lookupFrom(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := vars[key]
		return value, ok
	}
}

func actionEnvVars() map[string]string {
	return map[string]string{
		envHerdrEnv:      "1",
		envPluginID:      "example.layout",
		envPluginRoot:    "/plugins/example.layout",
		envConfigDir:     "/config/plugins/example.layout",
		envStateDir:      "/state/plugins/example.layout",
		envSocketPath:    "/run/herdr.sock",
		envBinPath:       "/usr/bin/herdr",
		envWorkspaceID:   "ws-1",
		envTabID:         "tab-1",
		envPaneID:        "pane-1",
		envActionID:      "apply",
		envContextJSON:   `{"workspace_id":"ws-1"}`,
		envClickedURL:    "https://example.test/issues/1",
		envLinkHandlerID: "github-issue",
	}
}

func TestLoadFrom(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		want    *Env
		wantErr error
	}{
		{
			name: "action with link handler",
			vars: actionEnvVars(),
			want: &Env{
				PluginID:      "example.layout",
				PluginRoot:    "/plugins/example.layout",
				ConfigDir:     "/config/plugins/example.layout",
				StateDir:      "/state/plugins/example.layout",
				SocketPath:    "/run/herdr.sock",
				BinPath:       "/usr/bin/herdr",
				WorkspaceID:   "ws-1",
				TabID:         "tab-1",
				PaneID:        "pane-1",
				ActionID:      "apply",
				ContextJSON:   []byte(`{"workspace_id":"ws-1"}`),
				ClickedURL:    "https://example.test/issues/1",
				LinkHandlerID: "github-issue",
			},
		},
		{
			name: "event hook",
			vars: map[string]string{
				envHerdrEnv:    "1",
				envPluginID:    "example.layout",
				envEvent:       "worktree.created",
				envEventJSON:   `{"event":"worktree_created","data":{"type":"worktree_created"}}`,
				envContextJSON: `{}`,
			},
			want: &Env{
				PluginID:    "example.layout",
				Event:       "worktree.created",
				EventJSON:   []byte(`{"event":"worktree_created","data":{"type":"worktree_created"}}`),
				ContextJSON: []byte(`{}`),
			},
		},
		{
			name: "pane without pane id",
			vars: map[string]string{
				envHerdrEnv:     "1",
				envPluginID:     "example.layout",
				envEntrypointID: "board",
			},
			want: &Env{PluginID: "example.layout", EntrypointID: "board"},
		},
		{
			name: "empty context json is not stored",
			vars: map[string]string{
				envHerdrEnv:    "1",
				envPluginID:    "example.layout",
				envContextJSON: "",
			},
			want: &Env{PluginID: "example.layout"},
		},
		{
			name:    "herdr env unset",
			vars:    map[string]string{envPluginID: "example.layout"},
			wantErr: ErrNotPluginProcess,
		},
		{
			name:    "herdr env not one",
			vars:    map[string]string{envHerdrEnv: "0", envPluginID: "example.layout"},
			wantErr: ErrNotPluginProcess,
		},
		{
			name:    "plugin id missing",
			vars:    map[string]string{envHerdrEnv: "1"},
			wantErr: ErrMissingPluginID,
		},
		{
			name:    "plugin id empty",
			vars:    map[string]string{envHerdrEnv: "1", envPluginID: ""},
			wantErr: ErrMissingPluginID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFrom(lookupFrom(tt.vars))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Fatalf("env = %+v, want nil", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("env = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadFromNilLookupUsesProcessEnvironment(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_PLUGIN_ID", "example.layout")
	t.Setenv("HERDR_PLUGIN_ENTRYPOINT_ID", "board")

	got, err := LoadFrom(nil)
	if err != nil {
		t.Fatalf("LoadFrom(nil) error = %v", err)
	}
	if got.PluginID != "example.layout" || got.EntrypointID != "board" {
		t.Errorf("env = %+v", got)
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		wantID  string
		wantErr error
	}{
		{
			name:   "startup hook",
			vars:   map[string]string{"HERDR_ENV": "1", "HERDR_PLUGIN_ID": "herdr.auto-title", "HERDR_PLUGIN_EVENT": "startup"},
			wantID: "herdr.auto-title",
		},
		{
			name:    "not a plugin process",
			vars:    map[string]string{"HERDR_PLUGIN_ID": "herdr.auto-title"},
			wantErr: ErrNotPluginProcess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"HERDR_ENV", "HERDR_PLUGIN_ID", "HERDR_PLUGIN_EVENT"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.vars {
				t.Setenv(key, value)
			}
			got, err := Load()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && got.PluginID != tt.wantID {
				t.Errorf("PluginID = %q, want %q", got.PluginID, tt.wantID)
			}
		})
	}
}

func TestEnvKind(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want EntryKind
	}{
		{name: "startup hook", env: Env{Event: "startup"}, want: KindStartup},
		{name: "startup hook wins over entrypoint id", env: Env{Event: "startup", EntrypointID: "board"}, want: KindStartup},
		{name: "action", env: Env{ActionID: "apply"}, want: KindAction},
		{name: "link handler action", env: Env{ActionID: "apply", ClickedURL: "https://example.test", LinkHandlerID: "github-issue"}, want: KindAction},
		{name: "action wins over event", env: Env{ActionID: "apply", Event: "pane.created"}, want: KindAction},
		{name: "event", env: Env{Event: "worktree.created"}, want: KindEvent},
		{name: "event with underscores after the dot", env: Env{Event: "pane.agent_status_changed"}, want: KindEvent},
		{name: "event wins over entrypoint id", env: Env{Event: "pane.created", EntrypointID: "board"}, want: KindEvent},
		{name: "pane", env: Env{EntrypointID: "board"}, want: KindPane},
		{name: "empty", env: Env{}, want: KindUnknown},
		{name: "event without a dot", env: Env{Event: "worktree_created"}, want: KindUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.Kind(); got != tt.want {
				t.Errorf("Kind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnvClient(t *testing.T) {
	env := &Env{SocketPath: "/run/herdr.sock"}
	if got := env.Client().SocketPath(); got != "/run/herdr.sock" {
		t.Errorf("SocketPath() = %q, want %q", got, "/run/herdr.sock")
	}
}
