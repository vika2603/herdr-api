package herdr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeRequest is one request line the fake server received.
type fakeRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`

	raw   []byte
	valid bool
}

// fakeServer mimics the Herdr API server: one request line per connection,
// one response line, then close. A handler that keeps writing after the
// response models a streaming method such as events.subscribe.
type fakeServer struct {
	t        *testing.T
	path     string
	listener net.Listener
	handle   func(*fakeSession)

	mu          sync.Mutex
	requests    []fakeRequest
	connections int
	clientBytes [][]byte
}

// fakeSession is one accepted connection together with its request.
type fakeSession struct {
	server *fakeServer
	conn   net.Conn
	reader *bufio.Reader
	req    fakeRequest
}

func newFakeServer(t *testing.T, handle func(*fakeSession)) *fakeServer {
	t.Helper()
	// A short directory keeps the socket path within the length limit that
	// macOS imposes on Unix domain sockets.
	dir, err := os.MkdirTemp("", "herdr")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	path := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on %s: %v", path, err)
	}
	server := &fakeServer{t: t, path: path, listener: listener, handle: handle}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.RemoveAll(dir)
	})
	go server.serve()
	return server
}

func (s *fakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serveConn(conn)
	}
}

func (s *fakeServer) serveConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}

	session := &fakeSession{server: s, conn: conn, reader: reader, req: parseFakeRequest(line)}
	s.mu.Lock()
	s.connections++
	s.requests = append(s.requests, session.req)
	s.mu.Unlock()

	if !session.req.valid {
		session.writeJSON(map[string]any{
			"id":    "",
			"error": map[string]string{"code": ErrCodeInvalidRequest, "message": "invalid request"},
		})
		return
	}
	s.handle(session)
}

func parseFakeRequest(line []byte) fakeRequest {
	req := fakeRequest{raw: line}
	req.valid = json.Unmarshal(bytes.TrimRight(line, "\r\n"), &req) == nil
	return req
}

// request returns the request received on connection index.
func (s *fakeServer) request(index int) fakeRequest {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if index >= len(s.requests) {
		s.t.Fatalf("no request recorded at index %d, have %d", index, len(s.requests))
	}
	return s.requests[index]
}

func (s *fakeServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

// clientWrites returns the bytes clients sent after their request line, as
// recorded by watchClientWrites.
func (s *fakeServer) clientWrites() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.clientBytes...)
}

func (s *fakeSession) request() fakeRequest { return s.req }

// writeString writes to the connection. Write errors mean the client is gone,
// which every caller here handles by simply stopping.
func (s *fakeSession) writeString(text string) {
	_, _ = io.WriteString(s.conn, text)
}

func (s *fakeSession) writeLine(text string) { s.writeString(text + "\n") }

func (s *fakeSession) writeJSON(value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		s.server.t.Errorf("encode response: %v", err)
		return
	}
	s.writeLine(string(encoded))
}

func (s *fakeSession) success(result any) {
	s.writeJSON(map[string]any{"id": s.req.ID, "result": result})
}

func (s *fakeSession) fail(code, message string) {
	s.writeJSON(map[string]any{"id": s.req.ID, "error": map[string]string{"code": code, "message": message}})
}

// watchClientWrites records anything the client sends after its request and
// closes the connection, the way herdr treats a write on a stream.
func (s *fakeSession) watchClientWrites() {
	go func() {
		buf := make([]byte, 512)
		n, err := s.reader.Read(buf)
		if n > 0 {
			s.server.mu.Lock()
			s.server.clientBytes = append(s.server.clientBytes, append([]byte(nil), buf[:n]...))
			s.server.mu.Unlock()
			_ = s.conn.Close()
			return
		}
		_ = err
	}()
}
