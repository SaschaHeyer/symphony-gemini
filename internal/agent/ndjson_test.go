package agent

import (
	"testing"
)

func TestFeed_CompleteLine(t *testing.T) {
	p := NewNdjsonParser()
	events := p.Feed([]byte(`{"type":"system","subtype":"init","session_id":"abc"}` + "\n"))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "system" {
		t.Errorf("expected Type=system, got %q", events[0].Type)
	}
	if events[0].Subtype != "init" {
		t.Errorf("expected Subtype=init, got %q", events[0].Subtype)
	}
}

func TestFeed_PartialLines(t *testing.T) {
	p := NewNdjsonParser()

	// First feed: partial line, no newline
	events1 := p.Feed([]byte(`{"type":"sys`))
	if len(events1) != 0 {
		t.Fatalf("expected 0 events from partial feed, got %d", len(events1))
	}

	// Second feed: rest of line with newline
	events2 := p.Feed([]byte(`tem","subtype":"init"}` + "\n"))
	if len(events2) != 1 {
		t.Fatalf("expected 1 event after completing line, got %d", len(events2))
	}
	if events2[0].Type != "system" {
		t.Errorf("expected Type=system, got %q", events2[0].Type)
	}
	if events2[0].Subtype != "init" {
		t.Errorf("expected Subtype=init, got %q", events2[0].Subtype)
	}
}

func TestFeed_MultipleLines(t *testing.T) {
	p := NewNdjsonParser()
	input := `{"type":"system","subtype":"init"}` + "\n" + `{"type":"assistant","subtype":"message"}` + "\n"
	events := p.Feed([]byte(input))

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "system" {
		t.Errorf("event[0]: expected Type=system, got %q", events[0].Type)
	}
	if events[1].Type != "assistant" {
		t.Errorf("event[1]: expected Type=assistant, got %q", events[1].Type)
	}
	if events[1].Subtype != "message" {
		t.Errorf("event[1]: expected Subtype=message, got %q", events[1].Subtype)
	}
}

func TestFeed_EmptyLines(t *testing.T) {
	p := NewNdjsonParser()
	events := p.Feed([]byte("\n\n" + `{"type":"system"}` + "\n\n"))

	if len(events) != 1 {
		t.Fatalf("expected 1 event (empties skipped), got %d", len(events))
	}
	if events[0].Type != "system" {
		t.Errorf("expected Type=system, got %q", events[0].Type)
	}
}

func TestFeed_MalformedJSON(t *testing.T) {
	p := NewNdjsonParser()
	events := p.Feed([]byte("not json\n" + `{"type":"system"}` + "\n"))

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "malformed" {
		t.Errorf("event[0]: expected Type=malformed, got %q", events[0].Type)
	}
	if raw, ok := events[0].Raw["raw"].(string); !ok || raw != "not json" {
		t.Errorf("event[0]: expected Raw[raw]=%q, got %v", "not json", events[0].Raw["raw"])
	}
	if events[1].Type != "system" {
		t.Errorf("event[1]: expected Type=system, got %q", events[1].Type)
	}
}

func TestFlush_RemainingBuffer(t *testing.T) {
	p := NewNdjsonParser()

	// Feed partial data without a trailing newline
	events := p.Feed([]byte(`{"type":"partial"`))
	if len(events) != 0 {
		t.Fatalf("expected 0 events from partial feed, got %d", len(events))
	}

	// Flush should emit the buffered data as a malformed event (incomplete JSON)
	flushed := p.Flush()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed event, got %d", len(flushed))
	}
	if flushed[0].Type != "malformed" {
		t.Errorf("expected Type=malformed, got %q", flushed[0].Type)
	}
}

func TestFlush_EmptyBuffer(t *testing.T) {
	p := NewNdjsonParser()
	events := p.Flush()

	if events != nil {
		t.Fatalf("expected nil from flush on empty buffer, got %v", events)
	}
}

func TestClassify_AllTypes(t *testing.T) {
	tests := []struct {
		name            string
		decoded         map[string]any
		wantEventType   string
		wantSubtype     string
	}{
		{
			name:          "system with init subtype",
			decoded:       map[string]any{"type": "system", "subtype": "init"},
			wantEventType: "system",
			wantSubtype:   "init",
		},
		{
			name:          "assistant with no subtype",
			decoded:       map[string]any{"type": "assistant"},
			wantEventType: "assistant",
			wantSubtype:   "",
		},
		{
			name:          "result with success subtype",
			decoded:       map[string]any{"type": "result", "subtype": "success"},
			wantEventType: "result",
			wantSubtype:   "success",
		},
		{
			name:          "result with error subtype",
			decoded:       map[string]any{"type": "result", "subtype": "error"},
			wantEventType: "result",
			wantSubtype:   "error",
		},
		{
			name:          "user type",
			decoded:       map[string]any{"type": "user"},
			wantEventType: "user",
			wantSubtype:   "",
		},
		{
			name:          "no type key defaults to unknown",
			decoded:       map[string]any{"some_other": "data"},
			wantEventType: "unknown",
			wantSubtype:   "",
		},
		{
			name:          "type is non-string defaults to unknown",
			decoded:       map[string]any{"type": 42},
			wantEventType: "unknown",
			wantSubtype:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType, subtype := classifyNdjsonEvent(tt.decoded)
			if eventType != tt.wantEventType {
				t.Errorf("classifyNdjsonEvent() eventType = %q, want %q", eventType, tt.wantEventType)
			}
			if subtype != tt.wantSubtype {
				t.Errorf("classifyNdjsonEvent() subtype = %q, want %q", subtype, tt.wantSubtype)
			}
		})
	}
}
