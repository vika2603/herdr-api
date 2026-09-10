//go:build !windows

package plugintest

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// listen opens the socket a Server answers on. The directory is short because
// macOS limits the length of a Unix domain socket path, which the temporary
// directory of a test with a long name can exceed.
func listen(t testing.TB) (net.Listener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "herdr")
	if err != nil {
		t.Fatalf("plugintest: create a socket directory: %v", err)
	}
	path := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("plugintest: listen on %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.RemoveAll(dir)
	})
	return listener, path
}
