# TEST SPEC: Claude Code Backend for Symphony Go

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-002

---

## 1. Unit Tests

### 1.1 NdjsonParser (`internal/agent/ndjson_test.go`)

| Test | Why |
|------|-----|
| `TestFeed_CompleteLine` | Verifies single complete JSON line is parsed and typed correctly |
| `TestFeed_PartialLines` | Core correctness: partial lines across Feed() calls must accumulate and parse once complete |
| `TestFeed_MultipleLines` | Verifies multiple events in one Feed() call are all returned |
| `TestFeed_EmptyLines` | Empty lines must be silently skipped, not cause errors |
| `TestFeed_MalformedJSON` | Invalid JSON must emit `malformed` event, not crash or lose subsequent events |
| `TestFlush_RemainingBuffer` | Flush returns buffered data as event (may be malformed if incomplete) |
| `TestFlush_EmptyBuffer` | Flush on empty buffer returns no events |
| `TestMapEventType_SystemInit` | system/init → session_started |
| `TestMapEventType_ResultSuccess` | result/success → turn_completed |
| `TestMapEventType_ResultError` | result/error → turn_failed |
| `TestMapEventType_ResultOther` | result/error_max_turns → turn_completed (turn is done regardless) |
| `TestMapEventType_AssistantToolUse` | assistant with tool_use content → tool_call |
| `TestMapEventType_AssistantText` | assistant without tool_use → notification |
| `TestMapEventType_Unknown` | unknown type → notification |

### 1.2 ClaudeRunner (`internal/agent/claude_runner_test.go`)

| Test | Why |
|------|-----|
| `TestBuildClaudeArgs_FirstTurn` | Verifies correct CLI args without --resume on first turn |
| `TestBuildClaudeArgs_WithResume` | Verifies --resume is appended when session ID exists |
| `TestBuildClaudeArgs_WithMcpConfig` | Verifies --mcp-config is appended when .mcp.json exists in workspace |
| `TestBuildClaudeArgs_NoMcpConfig` | Verifies --mcp-config is NOT appended when .mcp.json is absent |
| `TestBuildClaudeArgs_AllowedTools` | Verifies --allowedTools is added for each tool in config |
| `TestBuildClaudeArgs_PermissionMode` | Verifies --permission-mode flag is set |
| `TestReadSessionID_Exists` | Session ID file is read and trimmed correctly |
| `TestReadSessionID_Missing` | Missing file returns empty string (no error) |
| `TestWriteSessionID` | Session ID is written and can be read back |
| `TestExtractSessionID_FromInitEvent` | session_id extracted from system/init NDJSON event |
| `TestExtractUsage_FromResultEvent` | Token usage extracted from result event's usage field |
| `TestMapNdjsonToAgentEvent_AllTypes` | Each NDJSON event type maps to correct AgentEvent type |

### 1.3 Config (`internal/config/`)

| Test | Why |
|------|-----|
| `TestParseConfig_BackendDefault` | Omitted backend defaults to "gemini" |
| `TestParseConfig_BackendClaude` | backend: claude is accepted |
| `TestParseConfig_BackendInvalid` | Unknown backend fails validation |
| `TestParseConfig_ClaudeDefaults` | Missing claude section gets correct defaults |
| `TestParseConfig_ClaudeOverrides` | Partial claude section merges with defaults |
| `TestParseConfig_ClaudeCodeAlias` | "claude_code" key is aliased to "claude" |

### 1.4 Factory (`internal/agent/runner.go`)

| Test | Why |
|------|-----|
| `TestNewLauncher_Gemini` | "gemini" returns GeminiRunner |
| `TestNewLauncher_Claude` | "claude" returns ClaudeRunner |
| `TestNewLauncher_Empty` | "" returns GeminiRunner (default) |
| `TestNewLauncher_Invalid` | Unknown string returns error |

---

## 2. Integration Tests

### 2.1 ClaudeRunner Turn Loop (mock subprocess)

| Test | Why |
|------|-----|
| `TestClaudeRunner_SingleTurnSuccess` | E2E: spawn mock claude process, emit NDJSON, verify events reach orchestrator channel |
| `TestClaudeRunner_SessionPersistence` | Verify .symphony-session-id is written after turn 1 and --resume is used on turn 2 |
| `TestClaudeRunner_ProcessNonZeroExit` | Non-zero exit emits turn_failed and returns error |

**Mock strategy:** Create a small shell script or Go binary that outputs canned NDJSON to stdout and exits. Set `claude.command` to point to this mock. This avoids requiring real Claude CLI in tests.

---

## 3. E2E Critical Paths (Manual / CI with real CLI)

| Path | Why |
|------|-----|
| **Config → Launch → NDJSON → Events** | The golden path: WORKFLOW.md with `backend: claude` dispatches an issue, Claude CLI runs, NDJSON is parsed, events appear in dashboard |
| **Session resume across turns** | Turn 1 creates session, turn 2 resumes it. Verify conversation context is maintained. |

---

## 4. Not Tested (and why)

| Area | Reason |
|------|--------|
| Orchestrator dispatch/retry | Unchanged from SYMPHONY-GO-001; already tested |
| Workspace lifecycle | Unchanged; shared code path already tested |
| Prompt rendering | Unchanged; already tested |
| Real Claude API responses | External dependency; mock in unit tests, manual verify in E2E |
