package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vika2603/herdr-client"
	"github.com/vika2603/herdr-client/plugin"
)

func statusEvent(paneID, workspaceID string, status herdr.AgentStatus) *herdr.EventEnvelope {
	agent := "claude"
	return &herdr.EventEnvelope{
		Event: herdr.EventKindPaneAgentStatusChanged,
		Data: &herdr.PaneAgentStatusChangedEvent{
			PaneID:       paneID,
			WorkspaceID:  workspaceID,
			DisplayAgent: &agent,
			AgentStatus:  status,
		},
	}
}

func TestStartupAndEventHooks(t *testing.T) {
	env := &plugin.Env{StateDir: filepath.Join(t.TempDir(), "state")}
	ctx := context.Background()

	if err := onStartup(ctx, env); err != nil {
		t.Fatalf("onStartup() error = %v", err)
	}
	if err := onEvent(ctx, env, statusEvent("pane-1", "ws-1", herdr.AgentStatusWorking)); err != nil {
		t.Fatalf("onEvent() error = %v", err)
	}
	if err := onEvent(ctx, env, statusEvent("pane-2", "ws-2", herdr.AgentStatusBlocked)); err != nil {
		t.Fatalf("onEvent() error = %v", err)
	}

	records, err := readRecords(env)
	if err != nil {
		t.Fatalf("readRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want 2", records)
	}
	if records[0].PaneID != "pane-1" || records[0].Status != "working" || records[0].Agent != "claude" {
		t.Errorf("records[0] = %+v", records[0])
	}
	if records[1].WorkspaceID != "ws-2" || records[1].Status != "blocked" {
		t.Errorf("records[1] = %+v", records[1])
	}

	// A new session starts from an empty log.
	if err := onStartup(ctx, env); err != nil {
		t.Fatalf("onStartup() error = %v", err)
	}
	if records, err = readRecords(env); err != nil || len(records) != 0 {
		t.Errorf("readRecords() = %+v, %v, want no records", records, err)
	}
}

func TestEventHookRejectsAnotherEvent(t *testing.T) {
	env := &plugin.Env{StateDir: t.TempDir()}
	envelope := &herdr.EventEnvelope{
		Event: herdr.EventKindPaneCreated,
		Data:  &herdr.PaneCreatedEvent{Pane: herdr.PaneInfo{PaneID: "pane-1"}},
	}

	if err := onEvent(context.Background(), env, envelope); err == nil {
		t.Fatal("onEvent() error = nil, want an error naming the event")
	}
}

func TestHooksNeedAStateDirectory(t *testing.T) {
	env := &plugin.Env{}

	if err := onStartup(context.Background(), env); err == nil {
		t.Error("onStartup() error = nil, want one")
	}
	if err := onEvent(context.Background(), env, statusEvent("pane-1", "ws-1", herdr.AgentStatusIdle)); err == nil {
		t.Error("onEvent() error = nil, want one")
	}
	if err := onAction(context.Background(), env, actionShow); err == nil {
		t.Error("onAction() error = nil, want one")
	}
}

func TestActionRejectsAnUnknownID(t *testing.T) {
	env := &plugin.Env{StateDir: t.TempDir()}

	if err := onAction(context.Background(), env, "hide"); err == nil {
		t.Fatal("onAction() error = nil, want an error naming the action")
	}
}

func TestActionFiltersByInvokingWorkspace(t *testing.T) {
	env := &plugin.Env{
		StateDir:    t.TempDir(),
		ContextJSON: []byte(`{"workspace_id":"ws-2"}`),
	}
	ctx := context.Background()
	for _, event := range []*herdr.EventEnvelope{
		statusEvent("pane-1", "ws-1", herdr.AgentStatusWorking),
		statusEvent("pane-2", "ws-2", herdr.AgentStatusDone),
	} {
		if err := onEvent(ctx, env, event); err != nil {
			t.Fatalf("onEvent() error = %v", err)
		}
	}

	workspace, err := invokingWorkspace(env)
	if err != nil {
		t.Fatalf("invokingWorkspace() error = %v", err)
	}
	if workspace != "ws-2" {
		t.Errorf("invokingWorkspace() = %q, want %q", workspace, "ws-2")
	}
	if err := onAction(ctx, env, actionShow); err != nil {
		t.Errorf("onAction() error = %v", err)
	}
}

func TestInvokingWorkspaceWithoutAContext(t *testing.T) {
	tests := []struct {
		name string
		env  plugin.Env
		want string
	}{
		{name: "no context variable", env: plugin.Env{}},
		{name: "context without a workspace", env: plugin.Env{ContextJSON: []byte(`{}`)}},
		{name: "context with a workspace", env: plugin.Env{ContextJSON: []byte(`{"workspace_id":"ws-1"}`)}, want: "ws-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := invokingWorkspace(&tt.env)
			if err != nil {
				t.Fatalf("invokingWorkspace() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("invokingWorkspace() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadRecordsSkipsBrokenLines(t *testing.T) {
	env := &plugin.Env{StateDir: t.TempDir()}
	path := filepath.Join(env.StateDir, logName)
	content := "{\"pane_id\":\"pane-1\",\"status\":\"idle\"}\nnot json\n\n{\"pane_id\":\"pane-2\",\"status\":\"done\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	records, err := readRecords(env)
	if err != nil {
		t.Fatalf("readRecords() error = %v", err)
	}
	if len(records) != 2 || records[0].PaneID != "pane-1" || records[1].PaneID != "pane-2" {
		t.Errorf("records = %+v", records)
	}
}
