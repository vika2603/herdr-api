//go:build windows

package plugin

import "os"

// shutdownSignals on Windows is the interrupt Go can deliver; there is no
// SIGHUP, and a closed pane ends the process without a signal it can catch.
var shutdownSignals = []os.Signal{os.Interrupt}
