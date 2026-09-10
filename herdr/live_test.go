package herdr

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// newLiveClient returns a client for the server named by HERDR_SOCKET_PATH,
// or skips the test when no server is configured. Only read-only methods may
// be called against it.
func newLiveClient(t *testing.T) *Client {
	t.Helper()
	path := os.Getenv(envSocketPath)
	if path == "" {
		t.Skip("HERDR_SOCKET_PATH is not set")
	}
	return New(path, WithDialTimeout(2*time.Second))
}

func TestLivePing(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pong struct {
		Type     string `json:"type"`
		Version  string `json:"version"`
		Protocol uint32 `json:"protocol"`
	}
	if err := client.Call(ctx, "ping", nil, &pong); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if pong.Type != "pong" {
		t.Errorf("result type = %q, want pong", pong.Type)
	}
	if pong.Version == "" || pong.Protocol == 0 {
		t.Errorf("unexpected pong: %+v", pong)
	}
}

func TestLiveOpenStream(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]any{"subscriptions": []any{map[string]string{"type": "workspace.created"}}}
	stream, err := client.OpenStream(ctx, "events.subscribe", params)
	if err != nil {
		t.Fatalf("events.subscribe: %v", err)
	}
	defer func() { _ = stream.Close() }()

	var ack struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(stream.Ack(), &ack); err != nil {
		t.Fatalf("decode ack %s: %v", stream.Ack(), err)
	}
	if ack.Type != "subscription_started" {
		t.Errorf("ack type = %q, want subscription_started", ack.Type)
	}
}
