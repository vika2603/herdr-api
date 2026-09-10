package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Client sends requests to a Herdr server over its local socket.
//
// The server reads exactly one request per connection and closes the
// connection after writing the response, so every Call dials a fresh
// connection. Methods that keep the connection open, such as
// events.subscribe, go through OpenStream.
type Client struct {
	socketPath  string
	dialTimeout time.Duration
	nextID      func() string
}

// Option configures a Client.
type Option func(*Client)

// WithDialTimeout bounds the time spent connecting to the socket.
func WithDialTimeout(d time.Duration) Option {
	return func(c *Client) { c.dialTimeout = d }
}

// WithRequestIDs replaces the request id generator.
func WithRequestIDs(next func() string) Option {
	return func(c *Client) { c.nextID = next }
}

// New returns a Client that dials socketPath. It performs no I/O.
func New(socketPath string, opts ...Option) *Client {
	c := &Client{socketPath: socketPath}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewFromEnv resolves the socket path the way the herdr CLI does
// (HERDR_SOCKET_PATH, then HERDR_SESSION, then the default session) and
// returns a Client for it.
func NewFromEnv(opts ...Option) (*Client, error) {
	path, err := ResolveSocketPath("")
	if err != nil {
		return nil, err
	}
	return New(path, opts...), nil
}

// ResolveSocketPath returns the socket path for a named session, or, when
// session is empty, the path selected by HERDR_SOCKET_PATH, HERDR_SESSION and
// finally the default session.
func ResolveSocketPath(session string) (string, error) {
	_ = session
	return "", errNotImplemented
}

// SocketPath returns the path the Client dials.
func (c *Client) SocketPath() string { return c.socketPath }

// Call sends one request and decodes the response's result object into
// result, which may be nil. A server error response is returned as *Error.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	_, _, _, _ = ctx, method, params, result
	return errNotImplemented
}

// CallRaw sends one request and returns the raw result object.
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	_, _, _ = ctx, method, params
	return nil, errNotImplemented
}

// OpenStream sends one request, reads its acknowledging response, and keeps
// the connection open so that the lines the server pushes afterwards can be
// read from the returned Stream.
func (c *Client) OpenStream(ctx context.Context, method string, params any) (*Stream, error) {
	_, _, _ = ctx, method, params
	return nil, errNotImplemented
}

// Stream is a connection the server keeps open to push events.
type Stream struct{}

// RawEvent is one line pushed on an events.subscribe connection.
type RawEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// Ack returns the result object of the response that opened the stream.
func (s *Stream) Ack() json.RawMessage { return nil }

// Next blocks until the server pushes the next line or ctx is done.
func (s *Stream) Next(ctx context.Context) (*RawEvent, error) {
	_ = ctx
	return nil, errNotImplemented
}

// Close closes the connection. A blocked Next returns ErrStreamClosed.
func (s *Stream) Close() error { return nil }

// ErrStreamClosed is returned by Stream.Next once the connection is closed.
var ErrStreamClosed = errors.New("herdr: stream closed")

var errNotImplemented = errors.New("herdr: not implemented")

// Error is an error response from the server.
type Error struct {
	Method  string
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Method == "" {
		return e.Code + ": " + e.Message
	}
	return e.Method + ": " + e.Code + ": " + e.Message
}

// IsCode reports whether err is a server *Error carrying code.
func IsCode(err error, code string) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
