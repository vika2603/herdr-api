package herdr

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSubscribeDecodesEvents(t *testing.T) {
	server, lines := newStreamServer(t, subscriptionStarted)

	stream, err := New(server.path).Subscribe(context.Background(),
		PaneCreatedSubscription{},
		PaneScrollChangedSubscription{PaneID: "w1:p1"},
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = stream.Close() }()

	request := server.request(0)
	if request.Method != MethodEventsSubscribe {
		t.Errorf("method = %q, want %q", request.Method, MethodEventsSubscribe)
	}
	params := string(request.Params)
	for _, want := range []string{`"type":"pane.created"`, `"type":"pane.scroll_changed"`, `"pane_id":"w1:p1"`} {
		if !strings.Contains(params, want) {
			t.Errorf("params %s do not contain %s", params, want)
		}
	}

	lines <- `{"event":"pane_created","data":{"type":"pane_created","pane":{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","terminal_id":"t","agent_status":"idle","focused":true,"revision":3}}}`
	lines <- `{"event":"pane.scroll_changed","data":{"pane_id":"w1:p1","workspace_id":"w1","scroll":{"max_offset_from_bottom":10,"offset_from_bottom":2,"viewport_rows":40}}}`

	ctx := context.Background()
	first, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	created, ok := first.(*PaneCreatedEvent)
	if !ok {
		t.Fatalf("first event is %T, want *PaneCreatedEvent", first)
	}
	if created.Pane.PaneID != "w1:p1" || created.Pane.Revision != 3 {
		t.Errorf("pane = %+v", created.Pane)
	}

	second, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	scrolled, ok := second.(*PaneScrollChangedEvent)
	if !ok {
		t.Fatalf("second event is %T, want *PaneScrollChangedEvent", second)
	}
	if scrolled.Scroll.OffsetFromBottom != 2 {
		t.Errorf("scroll = %+v", scrolled.Scroll)
	}
}

func TestSubscribeRequiresSubscriptions(t *testing.T) {
	server, _ := newStreamServer(t, subscriptionStarted)

	if _, err := New(server.path).Subscribe(context.Background()); err == nil {
		t.Fatal("Subscribe without subscriptions succeeded")
	}
	if got := server.connectionCount(); got != 0 {
		t.Errorf("connections = %d, want 0", got)
	}
}

func TestSubscribeRejectsUnexpectedAck(t *testing.T) {
	server, _ := newStreamServer(t, `{"type":"pong","version":"0.9.0","protocol":22}`)

	_, err := New(server.path).Subscribe(context.Background(), PaneCreatedSubscription{})
	var unexpected *UnexpectedResultError
	if !errors.As(err, &unexpected) {
		t.Fatalf("Subscribe error = %v, want *UnexpectedResultError", err)
	}
	if unexpected.Want != "subscription_started" || unexpected.Got != "pong" {
		t.Errorf("error = %+v", unexpected)
	}
}

func TestSubscribeServerError(t *testing.T) {
	server := newFakeServer(t, func(s *fakeSession) {
		s.fail(ErrCodeStreamConflict, "another stream is active")
	})

	_, err := New(server.path).Subscribe(context.Background(), PaneCreatedSubscription{})
	if !IsCode(err, ErrCodeStreamConflict) {
		t.Fatalf("Subscribe error = %v, want %s", err, ErrCodeStreamConflict)
	}
}

func TestEventStreamNextReportsClosedStream(t *testing.T) {
	server, _ := newStreamServer(t, subscriptionStarted)

	stream, err := New(server.path).Subscribe(context.Background(), PaneCreatedSubscription{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("Next after Close = %v, want ErrStreamClosed", err)
	}
}
