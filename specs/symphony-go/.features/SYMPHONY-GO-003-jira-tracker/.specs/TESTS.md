# TEST SPEC: Jira Tracker Support

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-003

---

## 1. Unit Tests

### 1.1 JiraClient (`internal/tracker/jira_test.go`)

Mock strategy: `httptest.NewServer` (same pattern as existing `client_test.go`).

| Test | Why |
|------|-----|
| `TestJiraFetchCandidateIssues_CorrectJQL` | Verify JQL includes project key and status IN clause |
| `TestJiraFetchCandidateIssues_BasicAuth` | Verify Authorization header is `Basic <base64(email:token)>` |
| `TestJiraFetchCandidateIssues_Pagination` | Verify offset-based pagination collects all pages |
| `TestJiraFetchCandidateIssues_EmptyStates` | Empty activeStates → no HTTP call, empty result |
| `TestJiraFetchCandidateIssues_Normalization` | Full issue response → correct Issue struct fields |
| `TestJiraFetchIssueStatesByIDs_CorrectJQL` | Verify `key IN (...)` JQL for state refresh |
| `TestJiraFetchIssueStatesByIDs_EmptyIDs` | Empty IDs → no HTTP call, empty result |
| `TestJiraFetchIssuesByStates_Pagination` | Terminal state cleanup uses pagination |
| `TestJiraFetchIssuesByStates_EmptyStates` | Empty states → no HTTP call |
| `TestJiraClient_HTTP401` | 401 → `jira_auth_failed` error |
| `TestJiraClient_HTTP403` | 403 → `jira_forbidden` error |
| `TestJiraClient_HTTP400` | 400 → `jira_bad_request` error |
| `TestJiraClient_MalformedJSON` | Invalid JSON body → `jira_unknown_payload` |

### 1.2 Jira Normalization (`internal/tracker/jira_normalize_test.go`)

| Test | Why |
|------|-----|
| `TestNormalizeJiraIssue_FullFields` | All fields present → correct mapping |
| `TestNormalizeJiraIssue_MinimalFields` | Only required fields → nil optionals |
| `TestNormalizeJiraIssue_LabelsLowercase` | Labels are lowercased |
| `TestNormalizeJiraIssue_PriorityParsing` | Priority ID "2" → int 2 |
| `TestNormalizeJiraIssue_PriorityNil` | Missing priority → nil |
| `TestNormalizeJiraIssue_URL` | URL constructed as `<baseURL>/browse/<key>` |
| `TestNormalizeJiraIssue_BlockedBy` | Issue links with "is blocked by" → BlockedBy populated |
| `TestNormalizeJiraIssue_BlockedByIgnoresOutward` | Outward links (blocks) are not included |
| `TestExtractADFText_Simple` | Single paragraph → plain text |
| `TestExtractADFText_Nested` | Nested paragraphs + inline → concatenated text |
| `TestExtractADFText_String` | Plain string description (v2 fallback) → returned as-is |
| `TestExtractADFText_Nil` | nil description → empty string |
| `TestBuildJQL_BasicProject` | Simple project + states → correct JQL |
| `TestBuildJQL_QuotesInState` | State names with quotes are escaped |

### 1.3 Config (`internal/config/`)

| Test | Why |
|------|-----|
| `TestValidateDispatchConfig_JiraValid` | All Jira fields present → no error |
| `TestValidateDispatchConfig_JiraMissingEmail` | Missing email → error |
| `TestValidateDispatchConfig_JiraMissingEndpoint` | Missing endpoint → error |
| `TestParseConfig_TrackerEmail` | Email field parsed from YAML |
| `TestResolveConfig_JiraEnvFallbacks` | JIRA_API_TOKEN and JIRA_EMAIL env fallbacks work |

### 1.4 Tracker Factory (`internal/tracker/issue.go`)

| Test | Why |
|------|-----|
| `TestNewTrackerClient_Linear` | kind=linear → LinearClient |
| `TestNewTrackerClient_Jira` | kind=jira → JiraClient |
| `TestNewTrackerClient_Unknown` | unknown kind → error |

---

## 2. Integration Tests

| Test | Why |
|------|-----|
| `TestJiraClient_FullFetchCycle` | Mock server returns multi-page Jira response with links, verify all issues normalized and blockers extracted |

---

## 3. E2E Critical Paths (Manual)

| Path | Why |
|------|-----|
| **Jira Cloud → Dispatch → Agent** | Real Jira project, verify issues are fetched, dispatched, and agent runs |
| **State reconciliation** | Issue moved to Done in Jira → worker stops, workspace cleaned |

---

## 4. Not Tested (and why)

| Area | Reason |
|------|--------|
| Orchestrator dispatch/retry with Jira | Orchestrator is tracker-agnostic; already tested with Linear |
| Jira write operations | Out of scope — handled by agent via MCP |
| OAuth flow | Out of scope — only Basic Auth supported |
