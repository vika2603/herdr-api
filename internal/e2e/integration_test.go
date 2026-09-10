//go:build e2e

package e2e

import (
	"testing"

	"github.com/vika2603/herdr-client"
)

// stageIntegration lists the agent integrations the server knows. Installing
// one is out of reach, so the listing and the shape of its entries are what
// this call proves.
func stageIntegration(t *testing.T, h *harness, _ *state) {
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
