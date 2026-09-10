//go:build e2e

package e2e

import (
	"testing"

	"github.com/vika2603/herdr-api"
)

// stagePluginRegistry calls the plugin registry methods that answer without a
// linked plugin, and the integration listing. Linking a plugin and installing
// an integration are out of reach, so the registries are empty and the result
// type is what these calls prove.
func stagePluginRegistry(t *testing.T, h *harness, _ *state) {
	plugins, err := h.client.PluginList(h.ctx(t), herdr.PluginListParams{})
	if h.cover(t, herdr.MethodPluginList, plugins, err) && len(plugins.Plugins) > 0 {
		t.Errorf("plugin.list reports %d plugins although the suite links none", len(plugins.Plugins))
	}

	actions, err := h.client.PluginActionList(h.ctx(t), herdr.PluginActionListParams{})
	if h.cover(t, herdr.MethodPluginActionList, actions, err) && len(actions.Actions) > 0 {
		t.Errorf("plugin.action.list reports %d actions although no plugin is linked", len(actions.Actions))
	}

	logs, err := h.client.PluginLogList(h.ctx(t), herdr.PluginLogListParams{Limit: ptr(uint64(10))})
	if h.cover(t, herdr.MethodPluginLogList, logs, err) && len(logs.Logs) > 0 {
		t.Errorf("plugin.log.list reports %d entries although no plugin ran", len(logs.Logs))
	}

	integrations, err := h.client.IntegrationList(h.ctx(t))
	if h.cover(t, herdr.MethodIntegrationList, integrations, err) {
		if len(integrations.Integrations) == 0 {
			t.Errorf("integration.list reports no integration")
		}
		for _, integration := range integrations.Integrations {
			if integration.Target == "" || integration.State == "" {
				t.Errorf("integration.list returned an incomplete entry: %+v", integration)
			}
		}
	}
}
