# Symphony Go — Upgrade Plan

Learnings from OpenAI's Elixir WORKFLOW.md applied to our Go implementation.
Organized by priority: quick wins (prompt-only) first, then code changes.

---

## Phase 1: Prompt Upgrades (no code changes, hot-reload)

### 1.1 Status state machine in the prompt

**Current:** Agent leaves tickets in Todo. We tell it "don't leave in Todo" but give no structure.

**Upgrade:** Define an explicit status map and routing logic in the prompt.

```
Status map:
- Todo → immediately move to In Progress before starting work
- In Progress → implementation underway
- Human Review → PR attached, validated, waiting on human
- Done → terminal, no action
```

The agent should:
- On pickup: `Todo` → move to `In Progress` (via `gh` issue edit or Linear API curl)
- On completion: move to `Human Review` (not `Done` — let humans merge)
- On merge: human moves to `Done`

**Requires:** Either `gh` CLI with Linear integration, or a curl command to Linear's GraphQL API to transition states. Can be done with curl + `$LINEAR_API_KEY` in the prompt instructions.

### 1.2 Workpad comment on Linear issues

**Current:** No progress tracking visible in Linear. Only terminal logs.

**Upgrade:** Agent creates a single `## Workpad` comment on the Linear issue and updates it throughout execution with:
- Environment stamp (host, workspace path, git SHA)
- Hierarchical plan with checkboxes
- Acceptance criteria
- Validation results
- Notes with timestamps

**Implementation:** Add prompt instructions to:
1. On start: create or find a `## Workpad` comment via Linear API (curl)
2. On progress: update the same comment
3. On completion: final status in the comment

Template:
```markdown
## Workpad

`<hostname>:<workspace-path>@<short-sha>`

### Plan
- [ ] 1. Analyze codebase
- [ ] 2. Implement changes
- [ ] 3. Write tests
- [ ] 4. Commit, push, create PR

### Acceptance Criteria
- [ ] <from ticket description>

### Validation
- [ ] <targeted test or proof>

### Notes
- <timestamp> Started work
```

### 1.3 Better guardrails

**Current:** Minimal instructions.

**Upgrade:** Add to the prompt:
- "This is an unattended orchestration session. Never ask a human to perform follow-up actions."
- "Only stop early for a true blocker (missing auth/permissions). If blocked, record it in the workpad."
- "Final message must report completed actions and blockers only. Do not include 'next steps for user'."
- "Work only in the provided repository copy. Do not touch any other path."
- "If the branch PR is already closed/merged, create a fresh branch from origin/main and restart."
- "When meaningful out-of-scope improvements are discovered, file a separate Linear issue instead of expanding scope."

### 1.4 Continuation context

**Current:** Generic "This is retry attempt {{ attempt }}."

**Upgrade:**
```
{% if attempt %}
Continuation context:
- This is retry attempt #{{ attempt }} because the ticket is still in an active state.
- Resume from the current workspace state instead of restarting from scratch.
- Do not repeat already-completed investigation or validation.
- Do not end the turn while the issue remains in an active state unless blocked.
{% endif %}
```

### 1.5 PR feedback sweep

**Current:** Agent creates PR and stops.

**Upgrade:** Before moving to Human Review, add prompt instructions:
1. Read all PR comments: `gh pr view --comments`
2. Read inline review comments: `gh api repos/<owner>/<repo>/pulls/<pr>/comments`
3. Address every actionable comment (code change or explicit pushback reply)
4. Re-run validation after changes
5. Push updates
6. Repeat until no outstanding comments

### 1.6 Reproduction first

**Current:** Agent jumps straight to implementation.

**Upgrade:** Add to prompt:
- "Before implementing, capture a concrete reproduction signal (command output, file state, or behavior) and record it in the workpad."
- "Prefer a targeted proof that directly demonstrates the behavior you changed."

---

## Phase 2: WORKFLOW.md Config Upgrades (hot-reload)

### 2.1 Add active states for the full lifecycle

**Current:**
```yaml
active_states:
  - Todo
  - In Progress
```

**Upgrade:**
```yaml
active_states:
  - Todo
  - In Progress
  - Rework
```

This lets Symphony pick up tickets that reviewers push back to `Rework`.

### 2.2 Add `Rework` handling to prompt

When the agent picks up a `Rework` ticket:
1. Re-read the full issue body and all comments
2. Close the existing PR
3. Create a fresh branch from `origin/main`
4. Start over with a new workpad comment

### 2.3 Polling interval

**Current:** 30s

OpenAI uses 5s. Consider reducing for faster pickup:
```yaml
polling:
  interval_ms: 10000
```

---

## Phase 3: Code Changes (requires rebuild)

### 3.1 Linear GraphQL tool extension (EXT-2)

**Priority:** Medium — enables native Linear API access from the agent session.

**What:** Expose a `linear_graphql` client-side tool to the Gemini ACP session so the agent can:
- Transition issue states (`updateIssue(id, state)`)
- Create/update comments
- Attach PR URLs to issues
- Create follow-up issues

**Without this:** The agent can still use `curl` commands in the terminal to hit the Linear API, but it's clunky and requires the API key to be accessible in the workspace environment.

**Implementation:**
- Add the tool to ACP session capabilities during `initialize`
- Handle `item/tool/call` requests for `linear_graphql` in the ACP client
- Execute GraphQL against configured Linear auth
- Return results to the agent session

**Estimated effort:** ~200 lines in `internal/agent/acp.go` + `internal/tracker/client.go`

### 3.2 Pass LINEAR_API_KEY to workspace environment

**Priority:** High — easy win that unblocks curl-based Linear access.

**What:** Set `LINEAR_API_KEY` as an environment variable in the Gemini CLI subprocess so the agent can use `curl` to call Linear's API directly.

**Implementation:** In `NewACPClient`, add `cmd.Env` with the API key:
```go
cmd.Env = append(os.Environ(), "LINEAR_API_KEY="+apiKey)
```

**Estimated effort:** ~5 lines in `internal/agent/acp.go`

### 3.3 Set Gemini CLI to YOLO mode

**Priority:** High — eliminates permission prompts.

**What:** The `session/new` response shows Gemini CLI supports modes: `default`, `autoEdit`, `yolo`, `plan`. Currently we're in `default` mode which prompts for approval on every tool call.

**Implementation:** After `session/new`, send a `session/set_mode` request:
```json
{"method": "session/set_mode", "params": {"sessionId": "...", "modeId": "yolo"}}
```

This auto-approves all tools, eliminating the `session/request_permission` back-and-forth.

**Estimated effort:** ~10 lines in `internal/agent/session.go`

### 3.4 Richer dashboard

**Priority:** Low — nice-to-have.

**What:** Show more detail on the dashboard:
- Last message text (truncated) per running session
- Link to Linear issue URL
- Link to workspace path
- Recent events log (last 20 events)
- Per-issue detail page at `/issue/{identifier}`

**Estimated effort:** ~100 lines in `internal/server/dashboard.go`

### 3.5 Session log files

**Priority:** Medium — important for debugging.

**What:** Write all ACP messages for each session to a log file:
```
~/symphony_workspaces/.logs/AIE-7/session-<id>.jsonl
```

This gives full replay capability without flooding terminal output.

**Estimated effort:** ~30 lines in `internal/agent/acp.go`

---

## Phase 4: Advanced (future)

### 4.1 SSH worker support

Run agents on remote machines. Spec Appendix A.

### 4.2 Persistent retry queue

Survive restarts without losing retry state.

### 4.3 Multiple tracker support

Abstract the Linear client behind a tracker interface (already done) and add GitHub Issues, Jira, etc.

### 4.4 Cost tracking

Track estimated cost per issue based on token usage and model pricing.

---

## Recommended execution order

| # | Change | Type | Effort |
|---|---|---|---|
| 1 | Pass LINEAR_API_KEY to subprocess env | Code | 5 min |
| 2 | Set Gemini to YOLO mode | Code | 10 min |
| 3 | Status state machine in prompt | Prompt | 15 min |
| 4 | Workpad comment instructions | Prompt | 15 min |
| 5 | Better guardrails | Prompt | 10 min |
| 6 | Continuation context | Prompt | 5 min |
| 7 | PR feedback sweep | Prompt | 10 min |
| 8 | Add Rework to active_states | Config | 1 min |
| 9 | Session log files | Code | 30 min |
| 10 | Linear GraphQL tool | Code | 2 hrs |
| 11 | Richer dashboard | Code | 1 hr |

Items 1-8 can be done in one session. Items 1-2 need a rebuild; 3-8 are hot-reload.
