package plugintest

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"testing"

	"github.com/vika2603/herdr-client/herdr"
	"github.com/vika2603/herdr-client/plugin"
)

// Server is a Herdr API socket a test controls. It answers the calls a
// handler makes with scripted results and records what it was asked, so a
// handler that makes several calls can be tested as a whole rather than only
// up to its first request.
//
//	server := plugintest.NewServer(t).
//		Reply(herdr.MethodWorktreeCreate, herdr.WorktreeCreatedResponse{Worktree: …}).
//		Reply(herdr.MethodAgentStart, herdr.AgentStartedResponse{Agent: …})
//
//	if err := newPlugin().Dispatch(ctx, server.Env(plugintest.Action("open"))); err != nil {
//		t.Fatal(err)
//	}
//	if got := server.Methods(); !slices.Equal(got, want) {
//		t.Errorf("called %v, want %v", got, want)
//	}
//
// It answers one request per connection and closes, as Herdr does, so a
// handler that makes the same call twice needs no second script entry. A
// method with no entry is answered with an error naming it, which surfaces as
// a handler error rather than as a hang.
//
// Only events.subscribe and the graphics stream keep a connection open, and
// this server does not serve them: a plugin that mirrors the session is
// tested against its own rendering, not against a scripted stream.
//
// The server needs a Unix domain socket, so NewServer skips the test on
// Windows, where Herdr's API is a named pipe.
type Server struct {
	t        testing.TB
	path     string
	listener net.Listener

	mu       sync.Mutex
	replies  map[string]json.RawMessage
	failures map[string]herdr.Error
	calls    []Call
}

// Call is one request the server received.
type Call struct {
	Method string
	// Params is the request's params object, still encoded, so a test decodes
	// only the fields it asserts on.
	Params json.RawMessage
}

// NewServer starts a server that answers nothing yet and stops with the test.
func NewServer(t testing.TB) *Server {
	t.Helper()
	listener, path := listen(t)
	server := &Server{
		t:        t,
		path:     path,
		listener: listener,
		replies:  make(map[string]json.RawMessage),
		failures: make(map[string]herdr.Error),
	}
	go server.serve()
	return server
}

// Reply makes the server answer every call to method with result. The result
// type is what the generated wrapper for that method decodes, so a mismatched
// one surfaces in the handler as a decode error. A second Reply for the same
// method replaces the first.
func (s *Server) Reply(method string, result herdr.Result) *Server {
	s.t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		s.t.Fatalf("plugintest: encode the reply to %s: %v", method, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replies[method] = encoded
	delete(s.failures, method)
	return s
}

// Fail makes the server answer every call to method with an API error, which
// reaches the handler as *herdr.Error.
func (s *Server) Fail(method, code, message string) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[method] = herdr.Error{Code: code, Message: message}
	delete(s.replies, method)
	return s
}

// Env builds a plugin environment whose Client reaches this server, with the
// options Env applies.
func (s *Server) Env(opts ...Option) *plugin.Env {
	env := Env(opts...)
	env.SocketPath = s.path
	return env
}

// Calls returns the requests received, in the order they arrived.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// Methods returns the methods called, in order, which is usually the whole
// assertion a sequence test needs.
func (s *Server) Methods() []string {
	calls := s.Calls()
	methods := make([]string, len(calls))
	for i, call := range calls {
		methods[i] = call.Method
	}
	return methods
}

func (s *Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	var request struct {
		ID     string          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &request); err != nil {
		writeLine(conn, map[string]any{
			"id":    "",
			"error": map[string]string{"code": herdr.ErrCodeInvalidRequest, "message": "invalid request"},
		})
		return
	}

	s.mu.Lock()
	s.calls = append(s.calls, Call{Method: request.Method, Params: request.Params})
	result, replied := s.replies[request.Method]
	failure, failed := s.failures[request.Method]
	s.mu.Unlock()

	switch {
	case replied:
		writeLine(conn, map[string]any{"id": request.ID, "result": result})
	case failed:
		writeLine(conn, map[string]any{
			"id":    request.ID,
			"error": map[string]string{"code": failure.Code, "message": failure.Message},
		})
	default:
		writeLine(conn, map[string]any{
			"id": request.ID,
			"error": map[string]string{
				"code":    herdr.ErrCodeInvalidRequest,
				"message": "plugintest: no reply is scripted for " + request.Method,
			},
		})
	}
}

// writeLine writes one response line. A write error means the client is gone,
// which leaves nothing to report.
func writeLine(conn net.Conn, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(encoded, '\n'))
}
