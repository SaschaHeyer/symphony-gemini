# FUNCTIONAL SPEC: Jira Tracker Support

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-003

---

## 1. Overview

Add Jira Cloud as a second issue tracker alongside Linear. The user selects the tracker via `tracker.kind: jira` in WORKFLOW.md. Jira issues are fetched via the Jira Cloud REST API v3, normalized to the existing `Issue` struct, and flow through the same orchestrator pipeline.

### 1.1 Key Differences: Linear vs Jira

| Aspect | Linear | Jira Cloud |
|---|---|---|
| API | GraphQL | REST API v3 |
| Auth | `Authorization: <api_key>` | Basic Auth (`email:api_token` base64) |
| Project filter | `project.slugId` | JQL `project = <KEY>` |
| Issue key | `identifier` (e.g., `MT-123`) | `key` (e.g., `PROJ-123`) |
| States | `state.name` | `status.name` |
| Blockers | Relations with `type: "blocks"` | Issue links with `type.inward: "is blocked by"` |
| Branch name | `branchName` field | Not native (derived from key or custom field) |
| Pagination | Cursor-based (endCursor) | `startAt` + `maxResults` offset-based |
| Labels | On issue directly | On issue directly |
| Priority | Integer 1-4 | Object with `id` (string "1"-"5") |

### 1.2 What This Is NOT

- Not supporting Jira Server/Data Center (only Cloud REST API v3)
- Not supporting OAuth — only API token + email (Basic Auth)
- Not supporting custom JQL override (uses standard project + status filter)
- Not implementing write operations (state transitions are done by the agent via MCP tools)

---

## 2. Goals

1. **`tracker.kind: jira`** — A new tracker kind that uses Jira Cloud REST API v3.
2. **Same orchestrator pipeline** — Jira issues normalize to the same `Issue` struct. Dispatch, retry, reconciliation, and all workspace logic remain unchanged.
3. **Minimal config** — Only require `endpoint` (Jira base URL), `api_key` (API token), `email` (account email), and `project_slug` (Jira project key).
4. **Tracker factory** — main.go uses a factory to create the right `TrackerClient` based on `tracker.kind`.

---

## 3. Functional Requirements

### FR-1: Jira Configuration

**What:** Extend `TrackerConfig` with a Jira-specific `email` field and accept `tracker.kind: jira`.

**Acceptance Criteria:**
- AC-1.1: New field `tracker.email` (string) — required when `tracker.kind` is `"jira"`. Used for Basic Auth.
- AC-1.2: `tracker.endpoint` for Jira is the base URL (e.g., `https://yourcompany.atlassian.net`). Required when kind is `"jira"`.
- AC-1.3: `tracker.api_key` is the Jira API token. Required when kind is `"jira"`. Supports `$VAR` env resolution.
- AC-1.4: `tracker.email` supports `$VAR` env resolution. Also fallback to `JIRA_EMAIL` env var if empty.
- AC-1.5: `tracker.api_key` fallback to `JIRA_API_TOKEN` env var when kind is `"jira"` and api_key is empty.
- AC-1.6: `tracker.project_slug` for Jira is the Jira project key (e.g., `PROJ`). Required.
- AC-1.7: Default `active_states` for Jira: `["To Do", "In Progress"]`.
- AC-1.8: Default `terminal_states` for Jira: `["Done"]`.
- AC-1.9: Default `endpoint` for Jira: none (required, unlike Linear which has a default).

**Example WORKFLOW.md snippet:**
```yaml
tracker:
  kind: jira
  endpoint: https://mycompany.atlassian.net
  api_key: $JIRA_API_TOKEN
  email: $JIRA_EMAIL
  project_slug: PROJ
  active_states:
    - To Do
    - In Progress
  terminal_states:
    - Done
```

### FR-2: Jira Client (TrackerClient Implementation)

**What:** Implement `JiraClient` satisfying the `TrackerClient` interface using Jira Cloud REST API v3.

**Acceptance Criteria:**

#### Authentication
- AC-2.1: Use HTTP Basic Auth: base64-encode `email:api_token`, send as `Authorization: Basic <encoded>`.
- AC-2.2: Network timeout: 30 seconds (same as Linear).

#### FetchCandidateIssues(projectSlug, activeStates)
- AC-2.3: Use Jira REST API: `GET /rest/api/3/search` with JQL parameter.
- AC-2.4: JQL: `project = "<projectSlug>" AND status IN ("<state1>", "<state2>", ...) ORDER BY priority ASC, created ASC`.
- AC-2.5: Request fields: `key, summary, description, priority, status, labels, issuelinks, created, updated`.
- AC-2.6: Paginate with `startAt` and `maxResults` (page size 50).
- AC-2.7: Empty activeStates → return empty slice without API call.
- AC-2.8: Normalize each issue to the `Issue` struct (see FR-3).

#### FetchIssueStatesByIDs(ids)
- AC-2.9: Use JQL: `key IN ("<key1>", "<key2>", ...)` to fetch current states.
- AC-2.10: Note: Jira API accepts issue keys, not internal IDs, for JQL. The `Issue.ID` field will store the Jira issue key (e.g., `PROJ-123`) since that's the stable identifier.
- AC-2.11: Empty IDs → return empty slice without API call.
- AC-2.12: Request only fields needed: `key, summary, status`.

#### FetchIssuesByStates(projectSlug, states)
- AC-2.13: JQL: `project = "<projectSlug>" AND status IN ("<state1>", ...)`.
- AC-2.14: Paginate. Return minimal fields for cleanup.
- AC-2.15: Empty states → return empty slice.

#### Error Handling
- AC-2.16: Map Jira API errors to typed `TrackerError` categories:
  - HTTP 401 → `jira_auth_failed`
  - HTTP 403 → `jira_forbidden`
  - HTTP 404 → `jira_not_found`
  - HTTP 400 (bad JQL) → `jira_bad_request`
  - Other HTTP errors → `jira_api_status`
  - Network errors → `jira_api_request`
  - Parse errors → `jira_unknown_payload`

### FR-3: Jira Issue Normalization

**What:** Map Jira REST API issue fields to the existing `Issue` struct.

**Acceptance Criteria:**

| Issue field | Jira source | Notes |
|---|---|---|
| `ID` | `key` (e.g., `PROJ-123`) | Jira key is the stable identifier |
| `Identifier` | `key` (same as ID) | Used for workspace naming, display |
| `Title` | `fields.summary` | |
| `Description` | `fields.description` | Jira v3 uses ADF (Atlassian Document Format). Extract plain text. |
| `Priority` | `fields.priority.id` | Parse string to int. "1"=highest, "5"=lowest. Map to 1-4 range or pass through. nil if absent. |
| `State` | `fields.status.name` | |
| `BranchName` | nil | Jira has no native branch name field |
| `URL` | Constructed: `<endpoint>/browse/<key>` | |
| `Labels` | `fields.labels` | Already strings in Jira. Lowercase them. |
| `BlockedBy` | `fields.issuelinks` where `type.inward == "is blocked by"` and `inwardIssue` exists | Extract linked issue's key, state |
| `CreatedAt` | `fields.created` | ISO-8601 format |
| `UpdatedAt` | `fields.updated` | ISO-8601 format |

- AC-3.1: ADF description → plain text: extract text nodes recursively from the ADF JSON structure. If description is a string (API v2 fallback), use as-is.
- AC-3.2: Priority mapping: Jira priority IDs are strings ("1" through "5"). Parse to int. If absent or unparseable, set to nil.
- AC-3.3: Labels normalized to lowercase.
- AC-3.4: BlockedBy: filter `issuelinks` where `type.inward` contains "blocked by" (case-insensitive) and `inwardIssue` is present. Extract `inwardIssue.key` as Identifier and `inwardIssue.fields.status.name` as State.

### FR-4: Config Validation Updates

**What:** Update `ValidateDispatchConfig` to accept `tracker.kind: jira` and validate Jira-specific fields.

**Acceptance Criteria:**
- AC-4.1: `tracker.kind` accepts `"linear"` or `"jira"`.
- AC-4.2: When kind is `"jira"`: require `endpoint`, `api_key`, `email`, `project_slug`.
- AC-4.3: Missing `email` when kind is `"jira"` → error: `"tracker.email is required when tracker.kind is \"jira\""`.
- AC-4.4: Missing `endpoint` when kind is `"jira"` → error: `"tracker.endpoint is required when tracker.kind is \"jira\""`.

### FR-5: Tracker Factory

**What:** Replace hardcoded `NewLinearClient()` in main.go with a factory that creates the right client based on `tracker.kind`.

**Acceptance Criteria:**
- AC-5.1: `func NewTrackerClient(cfg *TrackerConfig) (TrackerClient, error)` — factory in tracker package.
- AC-5.2: `kind: "linear"` → `NewLinearClient(endpoint, apiKey)`.
- AC-5.3: `kind: "jira"` → `NewJiraClient(endpoint, email, apiKey)`.
- AC-5.4: Unknown kind → error.
- AC-5.5: main.go calls `NewTrackerClient(&resolved.Tracker)` instead of `NewLinearClient(...)`.

### FR-6: Config Resolution Updates

**What:** Extend `ResolveConfig` to resolve Jira-specific env vars.

**Acceptance Criteria:**
- AC-6.1: Resolve `$VAR` in `tracker.email`.
- AC-6.2: Fallback `tracker.email` to `JIRA_EMAIL` env var when kind is `"jira"` and email is empty after resolution.
- AC-6.3: Fallback `tracker.api_key` to `JIRA_API_TOKEN` env var when kind is `"jira"` and api_key is empty after `LINEAR_API_KEY` fallback.
- AC-6.4: Apply Jira-specific default states when kind is `"jira"` and states are not explicitly configured.

---

## 4. Out of Scope

- Jira Server / Data Center
- OAuth authentication
- Custom JQL override
- Write operations to Jira (state transitions done by agent via MCP)
- Custom field mapping
- Jira sprints / epics / boards
- Webhook-based updates (polling only)

---

## 5. Technical Constraints

- **API:** Jira Cloud REST API v3 (`/rest/api/3/`)
- **Auth:** Basic Auth with `email:api_token`
- **Description format:** Jira v3 uses ADF (Atlassian Document Format) for descriptions. Need a simple extractor for plain text.
- **Pagination:** Offset-based (`startAt` + `maxResults`), not cursor-based
- **Rate limits:** Jira Cloud has rate limits but no explicit headers for backoff. Standard retry on 429.

---

## 6. Example WORKFLOW.md (Jira + Claude Code)

```yaml
---
backend: claude
tracker:
  kind: jira
  endpoint: https://mycompany.atlassian.net
  api_key: $JIRA_API_TOKEN
  email: $JIRA_EMAIL
  project_slug: PROJ
  active_states:
    - To Do
    - In Progress
  terminal_states:
    - Done
claude:
  command: claude
  model: claude-sonnet-4-6
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/myorg/myrepo.git .
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
```
