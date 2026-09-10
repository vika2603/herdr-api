//go:build e2e

package e2e

import (
	"testing"

	"github.com/vika2603/herdr-api"
)

func stageServer(t *testing.T, h *harness, _ *state) {
	pong, err := h.client.Ping(h.ctx(t))
	if h.cover(t, herdr.MethodPing, pong, err) {
		if pong.Protocol != herdr.SchemaProtocol {
			t.Fatalf("server speaks protocol %d, the schema snapshot describes %d; the result table applies to the snapshot",
				pong.Protocol, herdr.SchemaProtocol)
		}
		if pong.Version == "" {
			t.Errorf("ping reported no version")
		}
	}

	snapshot, err := h.client.SessionSnapshot(h.ctx(t))
	if h.cover(t, herdr.MethodSessionSnapshot, snapshot, err) {
		if snapshot.Snapshot.Protocol != pong.Protocol {
			t.Errorf("session.snapshot reports protocol %d, ping reports %d",
				snapshot.Snapshot.Protocol, pong.Protocol)
		}
	}

	reload, err := h.client.ServerReloadConfig(h.ctx(t))
	if h.cover(t, herdr.MethodServerReloadConfig, reload, err) && reload.Status == "" {
		t.Errorf("server.reload_config reported no status")
	}

	manifests, err := h.client.ServerAgentManifests(h.ctx(t))
	if h.cover(t, herdr.MethodServerAgentManifests, manifests, err) && len(manifests.Manifests) == 0 {
		t.Errorf("server.agent_manifests reported no manifests")
	}

	reloaded, err := h.client.ServerReloadAgentManifests(h.ctx(t))
	if h.cover(t, herdr.MethodServerReloadAgentManifests, reloaded, err) && len(reloaded.Manifests) == 0 {
		t.Errorf("server.reload_agent_manifests reported no manifests")
	}

	// A headless server has no notification surface, so the response reports
	// why it was not shown; the result type is what is under test.
	shown, err := h.client.NotificationShow(h.ctx(t), herdr.NotificationShowParams{
		Title: "herdr-api e2e",
		Body:  ptr("end-to-end verification"),
		Sound: herdr.NotificationShowSoundNone,
	})
	if h.cover(t, herdr.MethodNotificationShow, shown, err) && shown.Shown && shown.Reason == "" {
		t.Errorf("notification.show reported neither a surface nor a reason")
	}

	title, err := h.client.ClientWindowTitleSet(h.ctx(t), herdr.ClientWindowTitleSetParams{Title: "herdr-api e2e"})
	if h.cover(t, herdr.MethodClientWindowTitleSet, title, err) && title.Reason == "" {
		t.Errorf("client.window_title.set reported no reason")
	}

	cleared, err := h.client.ClientWindowTitleClear(h.ctx(t))
	if h.cover(t, herdr.MethodClientWindowTitleClear, cleared, err) && cleared.Reason == "" {
		t.Errorf("client.window_title.clear reported no reason")
	}
}
