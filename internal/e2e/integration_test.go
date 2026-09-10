//go:build e2e

package e2e

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/vika2603/herdr-client/herdr"
)

// probeTarget is the agent whose integration the suite installs and removes.
// omp is chosen because herdr writes its extension into a directory the
// harness creates itself, so the run leaves nothing behind, and because it is
// the least likely of the supported agents to be installed for real.
const probeTarget = herdr.IntegrationTargetOmp

// stageIntegration lists the agent integrations, then installs and removes
// one inside the harness home.
//
// Integrations are written under the user's own home rather than under
// XDG_CONFIG_HOME, so these two methods were out of reach until the harness
// began redirecting HOME. The first install runs before the agent directory
// exists on purpose: herdr refuses and names the path it looked in, which
// proves the redirect took effect before anything is written.
func stageIntegration(t *testing.T, h *harness, _ *state) {
	integrations, err := h.client.IntegrationList(h.ctx(t))
	if !h.cover(t, herdr.MethodIntegrationList, integrations, err) {
		return
	}
	if len(integrations.Integrations) == 0 {
		t.Fatal("integration.list reports no integration")
	}
	for _, integration := range integrations.Integrations {
		if integration.Target == "" || integration.State == "" {
			t.Errorf("integration.list returned an incomplete entry: %+v", integration)
		}
	}
	if integrationState(t, h) != herdr.IntegrationStateNotInstalled {
		t.Fatalf("%s is already installed in the harness home", probeTarget)
	}

	dir := probeIntegrationDir(t, h)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create the %s extension directory: %v", probeTarget, err)
	}

	installed, err := h.client.IntegrationInstall(h.ctx(t), herdr.IntegrationInstallParams{Target: probeTarget})
	if h.cover(t, herdr.MethodIntegrationInstall, installed, err) {
		if got := integrationState(t, h); got != herdr.IntegrationStateCurrent {
			t.Errorf("%s is %q after integration.install, want %q",
				probeTarget, got, herdr.IntegrationStateCurrent)
		}
	}

	removed, err := h.client.IntegrationUninstall(h.ctx(t), herdr.IntegrationUninstallParams{Target: probeTarget})
	if h.cover(t, herdr.MethodIntegrationUninstall, removed, err) {
		if got := integrationState(t, h); got != herdr.IntegrationStateNotInstalled {
			t.Errorf("%s is %q after integration.uninstall, want %q",
				probeTarget, got, herdr.IntegrationStateNotInstalled)
		}
	}
}

// probeIntegrationDir asks the server where it would write the integration by
// letting the install fail, and returns the directory named in the refusal.
// Reading the path back rather than hard-coding it keeps the suite correct
// when herdr moves an agent's configuration, and stops the run before any
// write if the path is not inside the harness.
func probeIntegrationDir(t *testing.T, h *harness) string {
	t.Helper()
	_, err := h.client.IntegrationInstall(h.ctx(t), herdr.IntegrationInstallParams{Target: probeTarget})
	if err == nil {
		t.Fatalf("integration.install succeeded before the %s directory existed", probeTarget)
	}
	inRoot := regexp.MustCompile(regexp.QuoteMeta(h.root) + `[^\s"']*`)
	dir := strings.TrimRight(inRoot.FindString(err.Error()), ".")
	if dir == "" {
		t.Fatalf("integration.install looked outside the harness root %s: %v", h.root, err)
	}
	return dir
}

// integrationState reads the state integration.list reports for probeTarget.
func integrationState(t *testing.T, h *harness) herdr.IntegrationState {
	t.Helper()
	integrations, err := h.client.IntegrationList(h.ctx(t))
	if err != nil {
		t.Fatalf("integration.list: %v", err)
	}
	for _, integration := range integrations.Integrations {
		if integration.Target == probeTarget {
			return integration.State
		}
	}
	t.Fatalf("integration.list does not report %s", probeTarget)
	return ""
}
