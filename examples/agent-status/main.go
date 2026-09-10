// Command herdr-agent-status is a worked example of a Herdr plugin whose
// entrypoints are all served by one binary: plugin.Run picks the handler from
// the environment Herdr injects.
//
// The event hook appends one record per agent status change to a log in
// HERDR_PLUGIN_STATE_DIR, the startup hook starts a fresh log for each Herdr
// session, and the "show" action prints the log. See README.md.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vika2603/herdr-client"
	"github.com/vika2603/herdr-client/plugin"
)

// Entrypoints declared in herdr-plugin.toml. A test checks the manifest
// against these values.
const (
	actionShow = "show"
	hookEvent  = "pane.agent_status_changed"
)

// logName is the log file inside HERDR_PLUGIN_STATE_DIR.
const logName = "agent-status.jsonl"

// record is one line of the log.
type record struct {
	UnixMs      int64  `json:"unix_ms"`
	WorkspaceID string `json:"workspace_id"`
	PaneID      string `json:"pane_id"`
	Agent       string `json:"agent,omitempty"`
	Status      string `json:"status"`
}

func main() {
	os.Exit(plugin.Run(context.Background(), plugin.Handlers{
		Startup: onStartup,
		Action:  onAction,
		Event:   onEvent,
	}))
}

// onStartup empties the log so it only ever describes the running session.
func onStartup(_ context.Context, env *plugin.Env) error {
	path, err := logPath(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("reset %s: %w", path, err)
	}
	return nil
}

// onEvent appends the status change the hook was invoked for.
func onEvent(_ context.Context, env *plugin.Env, envelope *herdr.EventEnvelope) error {
	changed, ok := envelope.Data.(*herdr.PaneAgentStatusChangedEvent)
	if !ok {
		return fmt.Errorf("hook invoked for %q, which carries no agent status", envelope.Event)
	}
	entry := record{
		UnixMs:      time.Now().UnixMilli(),
		WorkspaceID: changed.WorkspaceID,
		PaneID:      changed.PaneID,
		Agent:       agentName(changed),
		Status:      string(changed.AgentStatus),
	}
	return appendRecord(env, entry)
}

// onAction prints the log, restricted to the workspace the action was invoked
// from when Herdr named one.
func onAction(_ context.Context, env *plugin.Env, id string) error {
	if id != actionShow {
		return fmt.Errorf("unknown action %q", id)
	}
	records, err := readRecords(env)
	if err != nil {
		return err
	}
	workspace, err := invokingWorkspace(env)
	if err != nil {
		return err
	}

	shown := 0
	for _, entry := range records {
		if workspace != "" && entry.WorkspaceID != workspace {
			continue
		}
		fmt.Printf("%s  %-8s  %-12s  %s\n",
			time.UnixMilli(entry.UnixMs).Format(time.TimeOnly), entry.Status, entry.PaneID, entry.Agent)
		shown++
	}
	if shown == 0 {
		fmt.Println("no agent status changes recorded in this session")
	}
	return nil
}

// invokingWorkspace returns the workspace the action was invoked from, or an
// empty string when Herdr passed no invocation context or no workspace in it.
func invokingWorkspace(env *plugin.Env) (string, error) {
	invocation, err := env.Context()
	if errors.Is(err, plugin.ErrNoContext) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if invocation.WorkspaceID == nil {
		return "", nil
	}
	return *invocation.WorkspaceID, nil
}

// agentName prefers the label Herdr displays over the detected agent id.
func agentName(changed *herdr.PaneAgentStatusChangedEvent) string {
	switch {
	case changed.DisplayAgent != nil:
		return *changed.DisplayAgent
	case changed.Agent != nil:
		return *changed.Agent
	default:
		return ""
	}
}

func logPath(env *plugin.Env) (string, error) {
	if env.StateDir == "" {
		return "", errors.New("HERDR_PLUGIN_STATE_DIR is not set")
	}
	return filepath.Join(env.StateDir, logName), nil
}

// appendRecord writes one JSON line. Herdr runs a hook process per event and
// several may overlap, so the record is written in a single append-mode write
// to keep concurrent lines from interleaving.
func appendRecord(env *plugin.Env, entry record) error {
	path, err := logPath(env)
	if err != nil {
		return err
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	_, writeErr := file.Write(append(line, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}

// readRecords reads the log. A missing log means no event hook has fired yet;
// a line that does not decode is skipped so one bad record cannot hide the
// rest.
func readRecords(env *plugin.Env) ([]record, error) {
	path, err := logPath(env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var records []record
	for _, line := range bytes.Split(data, []byte("\n")) {
		var entry record
		if len(line) == 0 || json.Unmarshal(line, &entry) != nil {
			continue
		}
		records = append(records, entry)
	}
	return records, nil
}
