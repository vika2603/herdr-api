package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

// ErrStreamClosed is returned by Stream.Next once the connection is closed.
var ErrStreamClosed = errors.New("herdr: stream closed")

// RawEvent is one line pushed on a streaming connection.
type RawEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// Stream is a connection the server keeps open to push events.
//
// Nothing is ever written to the connection after the opening request: the
// server closes a streaming connection as soon as the client sends anything
// else.
type Stream struct {
	method string
	conn   io.Closer
	ack    json.RawMessage

	// lines carries the pushed lines from the reading goroutine. It is closed
	// once that goroutine has reported a read error and stopped.
	lines  chan streamLine
	closed chan struct{}

	closeOnce sync.Once
	closeErr  error

	mu      sync.Mutex
	readErr error
}

type streamLine struct {
	data []byte
	err  error
}

func newStream(method string, conn io.Closer, r *bufio.Reader, ack json.RawMessage) *Stream {
	s := &Stream{
		method: method,
		conn:   conn,
		ack:    ack,
		lines:  make(chan streamLine),
		closed: make(chan struct{}),
	}
	go s.read(r)
	return s
}

// read hands every pushed line to Next. Reading on its own goroutine keeps a
// cancelled Next from having to close the connection.
func (s *Stream) read(r *bufio.Reader) {
	defer close(s.lines)
	for {
		line, err := r.ReadBytes('\n')
		if trimmed := bytes.TrimRight(line, "\r\n"); len(bytes.TrimSpace(trimmed)) > 0 {
			select {
			case s.lines <- streamLine{data: trimmed}:
			case <-s.closed:
				return
			}
		}
		if err != nil {
			select {
			case s.lines <- streamLine{err: err}:
			case <-s.closed:
			}
			return
		}
	}
}

// Ack returns the result object of the response that opened the stream.
func (s *Stream) Ack() json.RawMessage { return s.ack }

// Next blocks until the server pushes the next line, the stream is closed, or
// ctx is done. A closed stream reports ErrStreamClosed; a done context
// reports ctx.Err() and leaves the stream usable.
func (s *Stream) Next(ctx context.Context) (*RawEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-s.closed:
		return nil, s.terminalErr()
	default:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closed:
		return nil, s.terminalErr()
	case line, ok := <-s.lines:
		if !ok {
			return nil, s.terminalErr()
		}
		if line.err != nil {
			s.setReadErr(line.err)
			return nil, s.terminalErr()
		}
		event := &RawEvent{}
		if err := json.Unmarshal(line.data, event); err != nil {
			return nil, fmt.Errorf("herdr: %s: invalid event: %w", s.method, err)
		}
		return event, nil
	}
}

// Close closes the connection. A blocked Next returns ErrStreamClosed.
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.closeErr = s.conn.Close()
	})
	return s.closeErr
}

func (s *Stream) setReadErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr == nil {
		s.readErr = err
	}
}

// terminalErr reports the end of the stream as ErrStreamClosed, wrapping the
// read error when it says more than that the connection went away.
func (s *Stream) terminalErr() error {
	s.mu.Lock()
	err := s.readErr
	s.mu.Unlock()
	switch {
	case err == nil,
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, net.ErrClosed),
		errors.Is(err, os.ErrClosed):
		return ErrStreamClosed
	default:
		return fmt.Errorf("%w: %w", ErrStreamClosed, err)
	}
}
