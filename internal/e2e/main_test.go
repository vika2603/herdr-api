//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// methods are the method names the schema snapshot declares; table is the
// result type each of them is documented to answer with.
var (
	methods []string
	table   map[string]expectation
	suite   *harness
)

// TestMain owns the server: it starts one `herdr --session <name> server` on a
// temporary XDG_CONFIG_HOME, runs the suite against it, stops it and prints
// the coverage report.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("herdr"); err != nil {
		fmt.Println("e2e: skipping the suite: the herdr binary is not in PATH")
		os.Exit(0)
	}
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Println("e2e: skipping the suite: the git binary is not in PATH, so the worktree methods cannot be reached")
		os.Exit(0)
	}

	var err error
	if methods, err = schemaMethods(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}
	if table, err = methodResults(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}

	suite, err = newHarness()
	if err != nil {
		if suite != nil {
			suite.cleanup()
		}
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	suite.stop()
	writeReport(os.Stdout, methods, suite.rec.covered(), suite.rec.findings(), suite.version, suite.protocol)
	if code != 0 {
		fmt.Fprintf(os.Stderr, "e2e: server output:\n%s\n", suite.serverLog())
	}
	suite.cleanup()
	os.Exit(code)
}
