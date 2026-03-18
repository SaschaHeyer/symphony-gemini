# STEP-2: Jira Client & Normalization

**Goal:** Implement `JiraClient` satisfying `TrackerClient`, with REST API calls, pagination, normalization, and ADF text extraction.

---

## Files to Create

### `internal/tracker/jira.go`

**JiraClient struct:**
```go
type JiraClient struct {
    baseURL    string
    authHeader string
    httpClient *http.Client
}

func NewJiraClient(baseURL, email, apiToken string) *JiraClient
```

**Interface methods:**
- `FetchCandidateIssues(projectSlug string, activeStates []string) ([]Issue, error)`
- `FetchIssueStatesByIDs(ids []string) ([]Issue, error)`
- `FetchIssuesByStates(projectSlug string, states []string) ([]Issue, error)`

**Internal methods:**
- `search(jql, fields string, startAt, maxResults int) (*jiraSearchResponse, error)` — executes GET to `/rest/api/3/search`

**Helpers:**
- `buildJQL(projectSlug string, states []string) string` — builds JQL with proper quoting
- `buildKeyJQL(keys []string) string` — builds `key IN (...)` JQL
- `joinQuoted(values []string) string` — joins with quotes for JQL

**Jira response types (unexported):**
- `jiraSearchResponse`, `jiraIssue`, `jiraIssueFields`, `jiraPriority`, `jiraStatus`, `jiraIssueLink`, `jiraIssueLinkType`, `jiraLinkedIssue`

**Error mapping:**
- 401 → `ErrJiraAuthFailed`
- 403 → `ErrJiraForbidden`
- 404 → `ErrJiraNotFound`
- 400 → `ErrJiraBadRequest`
- Other non-200 → `ErrJiraAPIStatus`
- Network error → `ErrJiraAPIRequest`
- JSON parse error → `ErrJiraUnknownPayload`

### `internal/tracker/jira_normalize.go`

**Functions:**
- `normalizeJiraIssue(ji jiraIssue, baseURL string) Issue` — maps Jira fields to Issue struct
- `extractADFText(desc any) string` — extracts plain text from ADF or returns string as-is
- `extractADFTextFromNode(node map[string]any) string` — recursive ADF text extraction
- `lowercaseLabels(labels []string) []string` — lowercase label normalization

**Normalization rules:**
- `ID` = `Identifier` = Jira key (e.g., `PROJ-123`)
- `Title` = `fields.summary`
- `Description` = ADF → plain text
- `Priority` = `fields.priority.id` parsed to int, nil if absent
- `State` = `fields.status.name`
- `BranchName` = nil (not native in Jira)
- `URL` = `<baseURL>/browse/<key>`
- `Labels` = lowercased
- `BlockedBy` = issue links where `type.inward` contains "blocked by" (case-insensitive)
- `CreatedAt`/`UpdatedAt` = parsed from ISO-8601 (reuse `parseTimestamp` from normalize.go)

### `internal/tracker/errors.go` (modify)

Add error constants:
```go
const (
    ErrJiraAuthFailed     = "jira_auth_failed"
    ErrJiraForbidden      = "jira_forbidden"
    ErrJiraNotFound       = "jira_not_found"
    ErrJiraBadRequest     = "jira_bad_request"
    ErrJiraAPIStatus      = "jira_api_status"
    ErrJiraAPIRequest     = "jira_api_request"
    ErrJiraUnknownPayload = "jira_unknown_payload"
)
```

---

## Files to Create (Tests)

### `internal/tracker/jira_test.go`

Uses `httptest.NewServer` (same pattern as `client_test.go`).

| Test | What |
|------|------|
| `TestJiraFetchCandidateIssues_CorrectJQL` | Verify JQL query string in request |
| `TestJiraFetchCandidateIssues_BasicAuth` | Verify Authorization header format |
| `TestJiraFetchCandidateIssues_Pagination` | Multi-page response, all issues collected |
| `TestJiraFetchCandidateIssues_EmptyStates` | No HTTP call for empty states |
| `TestJiraFetchIssueStatesByIDs_CorrectJQL` | `key IN (...)` JQL |
| `TestJiraFetchIssueStatesByIDs_EmptyIDs` | No HTTP call for empty IDs |
| `TestJiraFetchIssuesByStates_EmptyStates` | No HTTP call for empty states |
| `TestJiraClient_HTTP401` | Returns jira_auth_failed |
| `TestJiraClient_HTTP400` | Returns jira_bad_request |
| `TestJiraClient_MalformedJSON` | Returns jira_unknown_payload |

### `internal/tracker/jira_normalize_test.go`

| Test | What |
|------|------|
| `TestNormalizeJiraIssue_FullFields` | All fields → correct mapping |
| `TestNormalizeJiraIssue_MinimalFields` | Only required fields → nil optionals |
| `TestNormalizeJiraIssue_LabelsLowercase` | Labels lowercased |
| `TestNormalizeJiraIssue_PriorityParsing` | "2" → int 2 |
| `TestNormalizeJiraIssue_BlockedBy` | Issue links filtered correctly |
| `TestNormalizeJiraIssue_URL` | URL construction |
| `TestExtractADFText_Simple` | Single paragraph |
| `TestExtractADFText_Nested` | Nested content |
| `TestExtractADFText_String` | Plain string (v2 fallback) |
| `TestExtractADFText_Nil` | nil → empty |
| `TestBuildJQL_Basic` | Project + states → JQL |
| `TestBuildJQL_QuoteEscaping` | Quotes in values escaped |

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./internal/tracker/ -run "TestJira"` — all new tests pass
- [ ] `go test ./internal/tracker/` — all existing tests still pass
