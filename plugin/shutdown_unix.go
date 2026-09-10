//go:build !windows

package plugin

import (
	"os"
	"syscall"
)

// shutdownSignals is what herdr 0.9.0 sends when a pane closes, plus the
// interrupt a terminal sends on its own.
var shutdownSignals = []os.Signal{syscall.SIGHUP, syscall.SIGTERM, os.Interrupt}
