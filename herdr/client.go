package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Client sends requests to a Herdr server over its local socket.
//
// The server reads exactly one request per connection and closes the
// connection after writing the response, so every Call dials a fresh
// connection. Methods that keep the connection open, such as
// events.subscribe, go through OpenStream.
//
// A Client is safe for concurrent use.
type Client struct {
	socketPath  string
	dialTimeout time.Duration
	nextID      func() string
	sequence    atomic.Uint64
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

// SocketPath returns the path the Client dials.
func (c *Client) SocketPath() string { return c.socketPath }

// Call sends one request and decodes the response's result object into
// result, which may be nil. A server error response is returned as *Error.
//
// Cancelling ctx or reaching its deadline closes the connection and the call
// reports ctx.Err().
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	raw, err := c.CallRaw(ctx, method, params)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("herdr: %s: cannot decode result: %w", method, err)
	}
	return nil
}

// CallRaw sends one request and returns the raw result object.
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	request, err := requestLine(c.requestID(), method, params)
	if err != nil {
		return nil, requestError(ctx, method, "cannot send request", err)
	}
	conn, err := dialSocket(ctx, c.socketPath, c.dialTimeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	stopWatch := watchContext(ctx, conn)
	defer stopWatch()

	if _, err := conn.Write(request); err != nil {
		return nil, requestError(ctx, method, "cannot send request", err)
	}
	line, err := readLine(bufio.NewReader(conn))
	if err != nil {
		return nil, requestError(ctx, method, "cannot read response", err)
	}
	return decodeResponseLine(method, line)
}

// OpenStream sends one request, reads its acknowledging response, and keeps
// the connection open so that the lines the server pushes afterwards can be
// read from the returned Stream.
//
// ctx bounds the opening request only. Once the Stream exists it lives until
// Close; each Next takes its own context.
func (c *Client) OpenStream(ctx context.Context, method string, params any) (*Stream, error) {
	request, err := requestLine(c.requestID(), method, params)
	if err != nil {
		return nil, requestError(ctx, method, "cannot send request", err)
	}
	conn, err := dialSocket(ctx, c.socketPath, c.dialTimeout)
	if err != nil {
		return nil, err
	}
	stopWatch := watchContext(ctx, conn)
	reader := bufio.NewReader(conn)

	ack, err := func() (json.RawMessage, error) {
		if _, err := conn.Write(request); err != nil {
			return nil, requestError(ctx, method, "cannot send request", err)
		}
		line, err := readLine(reader)
		if err != nil {
			return nil, requestError(ctx, method, "cannot read response", err)
		}
		return decodeResponseLine(method, line)
	}()
	stopWatch()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return newStream(method, conn, reader, ack), nil
}

// requestID returns the id of the next request.
func (c *Client) requestID() string {
	if c.nextID != nil {
		return c.nextID()
	}
	return fmt.Sprintf("herdr-go-%d", c.sequence.Add(1))
}

type wireRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type wireResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *wireError      `json:"error"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// emptyParams stands in for a nil params value; the server requires the field
// to be an object.
var emptyParams = json.RawMessage(`{}`)

// requestLine encodes one request as the line to write to the socket.
//
// Every caller encodes the line before it dials. The server reads the request
// once as soon as it accepts the connection and, finding nothing there yet,
// waits out a poll interval before looking again, so a request encoded on an
// open connection answers a poll interval later than one already in hand.
func requestLine(id, method string, params any) ([]byte, error) {
	if params == nil {
		params = emptyParams
	}
	line, err := json.Marshal(wireRequest{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

func decodeResponseLine(method string, line []byte) (json.RawMessage, error) {
	var response wireResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("herdr: %s: invalid response: %w", method, err)
	}
	if response.Error != nil {
		return nil, &Error{Method: method, Code: response.Error.Code, Message: response.Error.Message}
	}
	if len(response.Result) == 0 {
		return nil, fmt.Errorf("herdr: %s: response carries neither result nor error", method)
	}
	return response.Result, nil
}

// readLine reads one newline-terminated line without its line ending. Blank
// lines are skipped. A final line that the server did not terminate is
// returned rather than dropped.
func readLine(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(line)) > 0 {
			return line, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// requestError reports a failed exchange, preferring the context error when
// the connection was closed because ctx was done.
func requestError(ctx context.Context, method, what string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("herdr: %s: %s: %w", method, what, err)
}

// watchContext closes conn once ctx is done, which is the only way to unblock
// a read on a connection that has no deadline support on every platform. The
// returned function stops the watch and must run before conn outlives the
// call.
func watchContext(ctx context.Context, conn io.Closer) func() {
	done := ctx.Done()
	if done == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-done:
			_ = conn.Close()
		case <-stop:
		}
	}()
	return sync.OnceFunc(func() { close(stop) })
}
