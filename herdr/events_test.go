package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEventKindDotName(t *testing.T) {
	cases := map[EventKind]string{
		EventKindPaneCreated:              "pane.created",
		EventKindWorkspaceMetadataUpdated: "workspace.metadata_updated",
		EventKindPaneAgentStatusChanged:   "pane.agent_status_changed",
		EventKindLayoutUpdated:            "layout.updated",
	}
	for kind, want := range cases {
		if got := kind.DotName(); got != want {
			t.Errorf("%q.DotName() = %q, want %q", kind, got, want)
		}
	}
}

func TestDecodeLifecycleEvent(t *testing.T) {
	data := json.RawMessage(`{"type":"pane_closed","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1"}`)
	event, err := DecodeEvent("pane_closed", data)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	closed, ok := event.(*PaneClosedEvent)
	if !ok {
		t.Fatalf("event is %T, want *PaneClosedEvent", event)
	}
	if closed.PaneID != "w1:p1" || closed.WorkspaceID != "w1" {
		t.Errorf("event = %+v", closed)
	}
	if event.EventName() != "pane.closed" {
		t.Errorf("event name = %q, want pane.closed", event.EventName())
	}
}

func TestDecodeSubscriptionEvents(t *testing.T) {
	scroll, err := DecodeEvent("pane.scroll_changed", json.RawMessage(
		`{"pane_id":"w1:p1","workspace_id":"w1","scroll":{"offset_from_bottom":3,"max_offset_from_bottom":90,"viewport_rows":42}}`))
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	changed, ok := scroll.(*PaneScrollChangedEvent)
	if !ok {
		t.Fatalf("event is %T, want *PaneScrollChangedEvent", scroll)
	}
	if changed.Scroll.OffsetFromBottom != 3 || changed.Scroll.ViewportRows != 42 {
		t.Errorf("scroll = %+v", changed.Scroll)
	}
	if changed.EventName() != "pane.scroll_changed" {
		t.Errorf("event name = %q", changed.EventName())
	}

	matched, err := DecodeEvent("pane.output_matched", json.RawMessage(
		`{"pane_id":"w1:p1","matched_line":"done","read":{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","source":"recent","format":"text","text":"done\n","revision":7,"truncated":false}}`))
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	output, ok := matched.(*PaneOutputMatchedEvent)
	if !ok {
		t.Fatalf("event is %T, want *PaneOutputMatchedEvent", matched)
	}
	if output.MatchedLine != "done" || output.Read.Source != ReadSourceRecent || output.Read.Revision != 7 {
		t.Errorf("event = %+v", output)
	}
}

// TestDecodeAgentStatusChangedFromBothEnvelopes covers the payload that
// arrives both as a lifecycle event and under its dotted subscription name.
func TestDecodeAgentStatusChangedFromBothEnvelopes(t *testing.T) {
	lifecycle, err := DecodeEvent("pane_agent_status_changed", json.RawMessage(
		`{"type":"pane_agent_status_changed","pane_id":"w1:p1","workspace_id":"w1","agent_status":"working"}`))
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	subscription, err := DecodeEvent("pane.agent_status_changed", json.RawMessage(
		`{"pane_id":"w1:p1","workspace_id":"w1","agent_status":"working"}`))
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	first, ok := lifecycle.(*PaneAgentStatusChangedEvent)
	if !ok {
		t.Fatalf("lifecycle event is %T", lifecycle)
	}
	second, ok := subscription.(*PaneAgentStatusChangedEvent)
	if !ok {
		t.Fatalf("subscription event is %T", subscription)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("lifecycle %+v differs from subscription %+v", first, second)
	}
	if first.AgentStatus != AgentStatusWorking {
		t.Errorf("agent status = %q", first.AgentStatus)
	}
}

func TestDecodeEventRejectsUnknownNames(t *testing.T) {
	_, err := DecodeEvent("pane.something_new", json.RawMessage(`{"pane_id":"w1:p1"}`))
	var unknown *UnknownEventError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v (%T), want *UnknownEventError", err, err)
	}
	if unknown.Event != "pane.something_new" || string(unknown.Data) != `{"pane_id":"w1:p1"}` {
		t.Errorf("error = %+v", unknown)
	}

	_, err = DecodeEvent("something_new", json.RawMessage(`{"type":"something_new"}`))
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v (%T), want *UnknownEventError", err, err)
	}
}

func TestEventEnvelopeUnmarshal(t *testing.T) {
	line := []byte(`{"event":"tab_focused","data":{"type":"tab_focused","tab_id":"w1:t2","workspace_id":"w1"}}`)
	var envelope EventEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if envelope.Event != EventKindTabFocused {
		t.Errorf("event = %q, want %q", envelope.Event, EventKindTabFocused)
	}
	focused, ok := envelope.Data.(*TabFocusedEvent)
	if !ok {
		t.Fatalf("data is %T, want *TabFocusedEvent", envelope.Data)
	}
	if focused.TabID != "w1:t2" || focused.WorkspaceID != "w1" {
		t.Errorf("data = %+v", focused)
	}

	// Marshalling writes the discriminator back, so the line decodes again.
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var again EventEnvelope
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatalf("Unmarshal after Marshal: %s: %v", encoded, err)
	}
	if again.Event != envelope.Event {
		t.Errorf("event = %q, want %q", again.Event, envelope.Event)
	}
	if !reflect.DeepEqual(again.Data, envelope.Data) {
		t.Errorf("data = %+v, want %+v", again.Data, envelope.Data)
	}
}

func TestStreamNextEvent(t *testing.T) {
	server, lines := newStreamServer(t, subscriptionStarted)
	stream := openTestStream(t, server)

	lines <- `{"event":"pane_focused","data":{"type":"pane_focused","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1"}}`
	lines <- `{"event":"pane.scroll_changed","data":{"pane_id":"w1:p1","workspace_id":"w1","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":10,"viewport_rows":40}}}`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := stream.NextEvent(ctx)
	if err != nil {
		t.Fatalf("NextEvent: %v", err)
	}
	if _, ok := first.(*PaneFocusedEvent); !ok {
		t.Errorf("first event is %T, want *PaneFocusedEvent", first)
	}
	if first.EventName() != "pane.focused" {
		t.Errorf("event name = %q, want pane.focused", first.EventName())
	}

	second, err := stream.NextEvent(ctx)
	if err != nil {
		t.Fatalf("NextEvent: %v", err)
	}
	if _, ok := second.(*PaneScrollChangedEvent); !ok {
		t.Errorf("second event is %T, want *PaneScrollChangedEvent", second)
	}
}
