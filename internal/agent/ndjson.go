package agent

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NdjsonEvent represents a parsed event from Claude Code's stream-json output.
type NdjsonEvent struct {
	Type    string         // "system", "assistant", "result", "user", etc.
	Subtype string         // "init", "success", "error", etc.
	Raw     map[string]any // Full decoded JSON object
}

// NdjsonParser is a stateful line-accumulator for NDJSON streams.
type NdjsonParser struct {
	buffer []byte
}

// NewNdjsonParser returns a parser with an empty buffer.
func NewNdjsonParser() *NdjsonParser {
	return &NdjsonParser{}
}

// Feed appends data to the internal buffer, splits on newline boundaries,
// and returns parsed events for each complete line.
// Incomplete trailing data remains buffered for the next Feed call.
func (p *NdjsonParser) Feed(data []byte) []NdjsonEvent {
	p.buffer = append(p.buffer, data...)

	lines, remainder := splitLines(p.buffer)
	p.buffer = remainder

	var events []NdjsonEvent
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}
		events = append(events, parseLine(trimmed))
	}
	return events
}

// Flush drains any remaining buffered data as a final event.
// Returns nil if the buffer is empty.
func (p *NdjsonParser) Flush() []NdjsonEvent {
	if len(p.buffer) == 0 {
		return nil
	}

	trimmed := strings.TrimSpace(string(p.buffer))
	p.buffer = nil

	if trimmed == "" {
		return nil
	}

	return []NdjsonEvent{parseLine(trimmed)}
}

// parseLine decodes a single JSON line into an NdjsonEvent.
func parseLine(line string) NdjsonEvent {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		return NdjsonEvent{
			Type: "malformed",
			Raw:  map[string]any{"raw": line},
		}
	}

	eventType, subtype := classifyNdjsonEvent(decoded)
	return NdjsonEvent{
		Type:    eventType,
		Subtype: subtype,
		Raw:     decoded,
	}
}

// splitLines splits data on '\n' boundaries. The last element (after the final
// newline, or a partial line with no trailing newline) is returned as the remainder.
func splitLines(data []byte) (lines [][]byte, remainder []byte) {
	parts := bytes.Split(data, []byte("\n"))
	if len(parts) == 0 {
		return nil, nil
	}
	return parts[:len(parts)-1], parts[len(parts)-1]
}

// classifyNdjsonEvent extracts the "type" and "subtype" strings from a decoded
// JSON map. If no "type" key is present, eventType defaults to "unknown".
func classifyNdjsonEvent(decoded map[string]any) (eventType, subtype string) {
	if t, ok := decoded["type"].(string); ok {
		eventType = t
	} else {
		eventType = "unknown"
	}

	if s, ok := decoded["subtype"].(string); ok {
		subtype = s
	}

	return eventType, subtype
}
