# STEP-1: NDJSON Parser

**Goal:** Create a reusable, stateful line-accumulator that buffers partial NDJSON reads and emits typed events.

---

## Files to Create

### `internal/agent/ndjson.go`

**Structs:**

```go
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
```

**Functions:**

1. `NewNdjsonParser() *NdjsonParser` — returns parser with empty buffer.

2. `(p *NdjsonParser) Feed(data []byte) []NdjsonEvent`:
   - Append `data` to `p.buffer`.
   - Split on `\n` boundaries.
   - For each complete line: trim whitespace, skip empty, JSON decode.
   - On decode success: extract `type` and `subtype` from decoded map, return `NdjsonEvent`.
   - On decode failure: return `NdjsonEvent{Type: "malformed", Raw: map[string]any{"raw": trimmedLine}}`.
   - Remaining incomplete line stays in `p.buffer`.

3. `(p *NdjsonParser) Flush() []NdjsonEvent`:
   - If buffer is non-empty, attempt to decode it.
   - Return result (may be malformed). Clear buffer.
   - If buffer is empty, return nil.

**Helpers (unexported):**

4. `splitLines(data []byte) (lines [][]byte, remainder []byte)`:
   - Split on `\n`. Last element (after final newline or partial) is remainder.

5. `classifyNdjsonEvent(decoded map[string]any) (eventType, subtype string)`:
   - Read `decoded["type"]` as string → eventType.
   - Read `decoded["subtype"]` as string → subtype.
   - If no `type` key → eventType = "unknown".

---

### `internal/agent/ndjson_test.go`

**Tests (table-driven where possible):**

| Test Function | Input | Expected |
|---|---|---|
| `TestFeed_CompleteLine` | `{"type":"system","subtype":"init","session_id":"abc"}\n` | 1 event: Type="system", Subtype="init" |
| `TestFeed_PartialLines` | Feed 1: `{"type":"sys` / Feed 2: `tem","subtype":"init"}\n` | Feed 1: 0 events / Feed 2: 1 event |
| `TestFeed_MultipleLines` | Two complete JSON lines in one Feed | 2 events |
| `TestFeed_EmptyLines` | `\n\n{"type":"system"}\n\n` | 1 event (empties skipped) |
| `TestFeed_MalformedJSON` | `not json\n{"type":"system"}\n` | 2 events: first malformed, second system |
| `TestFlush_RemainingBuffer` | Feed partial, then Flush | 1 malformed event |
| `TestFlush_EmptyBuffer` | Flush on fresh parser | 0 events |
| `TestClassify_AllTypes` | Various `type`/`subtype` combos | Correct Type/Subtype strings |

---

## DoD (Definition of Done)

- [ ] `go build ./...` passes
- [ ] `go test ./internal/agent/ -run TestFeed` passes (all NDJSON tests)
- [ ] `go test ./internal/agent/ -run TestFlush` passes
- [ ] `go test ./internal/agent/ -run TestClassify` passes
- [ ] No changes to existing files
