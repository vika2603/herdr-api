//go:build windows

package plugintest

import (
	"net"
	"testing"
)

// listen skips the test. Herdr's API on Windows is a named pipe, and creating
// one requires CreateNamedPipe, which the standard library does not expose;
// a Unix domain socket would not be reachable by the client's Windows dialer.
func listen(t testing.TB) (net.Listener, string) {
	t.Helper()
	t.Skip("plugintest: Server needs a Unix domain socket, which Windows does not offer for the Herdr API")
	return nil, ""
}
