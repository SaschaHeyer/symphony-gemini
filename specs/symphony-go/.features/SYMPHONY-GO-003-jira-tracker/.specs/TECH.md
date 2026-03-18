# TECH SPEC: Jira Tracker Support

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-003

---

## 1. Architecture

```
main.go
  │
  ├─ config.ParseConfig()              ── reads tracker.kind + email
  ├─ config.ResolveConfig()            ── resolves $JIRA_API_TOKEN, $JIRA_EMAIL
  ├─ config.ValidateDispatchConfig()   ── validates jira-specific fields
  │
  ├─ tracker.NewTrackerClient(&cfg)    ── factory returns LinearClient or JiraClient
  │
  └─ orchestrator.New(cfg, wf, trackerClient, launcher, wsMgr)
       │
       ├─ [kind=linear] LinearClient   ── GraphQL (unchanged)
       └─ [kind=jira]   JiraClient     ── REST API v3
```

---

## 2. New Files

| File | Purpose |
|------|---------|
| `internal/tracker/jira.go` | `JiraClient` implementing `TrackerClient` — REST API calls, pagination, auth |
| `internal/tracker/jira_normalize.go` | Jira issue → `Issue` normalization, ADF text extraction |
| `internal/tracker/jira_test.go` | Unit tests with mock HTTP server |
| `internal/tracker/jira_normalize_test.go` | Normalization + ADF extraction tests |

## 3. Modified Files

| File | Changes |
|------|---------|
| `internal/tracker/issue.go` | Add `NewTrackerClient()` factory |
| `internal/tracker/errors.go` | Add Jira error kind constants |
| `internal/config/config.go` | Add `Email` field to `TrackerConfig` |
| `internal/config/defaults.go` | No changes (no default endpoint for Jira) |
| `internal/config/resolve.go` | Resolve `$VAR` in email, Jira env fallbacks |
| `internal/config/validate.go` | Accept `"jira"`, validate email + endpoint |
| `cmd/symphony/main.go` | Use `NewTrackerClient()` factory |

---

## 4. Data Models

### 4.1 TrackerConfig Update

```go
// internal/config/config.go — add Email field

type TrackerConfig struct {
    Kind           string   `yaml:"kind"            json:"kind"`
    Endpoint       string   `yaml:"endpoint"        json:"endpoint"`
    APIKey         string   `yaml:"api_key"         json:"api_key"`
    Email          string   `yaml:"email"           json:"email"`      // NEW — required for Jira
    ProjectSlug    string   `yaml:"project_slug"    json:"project_slug"`
    ActiveStates   []string `yaml:"active_states"   json:"active_states"`
    TerminalStates []string `yaml:"terminal_states" json:"terminal_states"`
}
```

### 4.2 JiraClient

```go
// internal/tracker/jira.go

type JiraClient struct {
    baseURL    string       // e.g., "https://mycompany.atlassian.net"
    authHeader string       // "Basic <base64(email:token)>"
    httpClient *http.Client
}

func NewJiraClient(baseURL, email, apiToken string) *JiraClient
```

### 4.3 Jira API Response Structures (internal, unexported)

```go
// Jira search response
type jiraSearchResponse struct {
    Issues     []jiraIssue `json:"issues"`
    StartAt    int         `json:"startAt"`
    MaxResults int         `json:"maxResults"`
    Total      int         `json:"total"`
}

type jiraIssue struct {
    Key    string          `json:"key"`
    Fields jiraIssueFields `json:"fields"`
}

type jiraIssueFields struct {
    Summary     string           `json:"summary"`
    Description any              `json:"description"` // ADF object or string
    Priority    *jiraPriority    `json:"priority"`
    Status      jiraStatus       `json:"status"`
    Labels      []string         `json:"labels"`
    IssueLinks  []jiraIssueLink  `json:"issuelinks"`
    Created     string           `json:"created"`
    Updated     string           `json:"updated"`
}

type jiraPriority struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type jiraStatus struct {
    Name string `json:"name"`
}

type jiraIssueLink struct {
    Type         jiraIssueLinkType `json:"type"`
    InwardIssue  *jiraLinkedIssue  `json:"inwardIssue"`
    OutwardIssue *jiraLinkedIssue  `json:"outwardIssue"`
}

type jiraIssueLinkType struct {
    Inward  string `json:"inward"`
    Outward string `json:"outward"`
}

type jiraLinkedIssue struct {
    Key    string `json:"key"`
    Fields struct {
        Status jiraStatus `json:"status"`
    } `json:"fields"`
}
```

---

## 5. JiraClient Implementation Detail

### 5.1 Authentication

```go
func NewJiraClient(baseURL, email, apiToken string) *JiraClient {
    auth := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
    return &JiraClient{
        baseURL:    strings.TrimRight(baseURL, "/"),
        authHeader: "Basic " + auth,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}
```

### 5.2 FetchCandidateIssues

```go
func (c *JiraClient) FetchCandidateIssues(projectSlug string, activeStates []string) ([]Issue, error) {
    if len(activeStates) == 0 {
        return nil, nil
    }

    jql := buildJQL(projectSlug, activeStates)
    fields := "key,summary,description,priority,status,labels,issuelinks,created,updated"

    var allIssues []Issue
    startAt := 0
    for {
        resp, err := c.search(jql, fields, startAt, 50)
        // ... normalize each issue, append
        // ... break when startAt + len >= total
    }
    return allIssues, nil
}
```

### 5.3 JQL Builder

```go
func buildJQL(projectSlug string, states []string) string {
    // project = "PROJ" AND status IN ("To Do", "In Progress") ORDER BY priority ASC, created ASC
    quoted := make([]string, len(states))
    for i, s := range states {
        quoted[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
    }
    return fmt.Sprintf(
        `project = "%s" AND status IN (%s) ORDER BY priority ASC, created ASC`,
        strings.ReplaceAll(projectSlug, `"`, `\"`),
        strings.Join(quoted, ", "),
    )
}
```

### 5.4 REST API Call

```go
func (c *JiraClient) search(jql, fields string, startAt, maxResults int) (*jiraSearchResponse, error) {
    u := fmt.Sprintf("%s/rest/api/3/search?jql=%s&fields=%s&startAt=%d&maxResults=%d",
        c.baseURL,
        url.QueryEscape(jql),
        url.QueryEscape(fields),
        startAt,
        maxResults,
    )

    req, _ := http.NewRequest("GET", u, nil)
    req.Header.Set("Authorization", c.authHeader)
    req.Header.Set("Accept", "application/json")

    resp, err := c.httpClient.Do(req)
    // ... check status, parse JSON
}
```

### 5.5 FetchIssueStatesByIDs

```go
func (c *JiraClient) FetchIssueStatesByIDs(ids []string) ([]Issue, error) {
    if len(ids) == 0 {
        return nil, nil
    }

    // ids contain Jira keys like "PROJ-123"
    jql := fmt.Sprintf(`key IN (%s)`, joinQuoted(ids))
    fields := "key,summary,status"
    // ... single page fetch, normalize minimally
}
```

### 5.6 FetchIssuesByStates

Same pattern as FetchCandidateIssues but with different states and minimal fields.

---

## 6. Jira Issue Normalization

```go
// internal/tracker/jira_normalize.go

func normalizeJiraIssue(ji jiraIssue, baseURL string) Issue {
    issue := Issue{
        ID:         ji.Key,           // PROJ-123
        Identifier: ji.Key,           // PROJ-123
        Title:      ji.Fields.Summary,
        State:      ji.Fields.Status.Name,
        Labels:     lowercaseLabels(ji.Fields.Labels),
    }

    // Description: ADF → plain text
    if ji.Fields.Description != nil {
        text := extractADFText(ji.Fields.Description)
        if text != "" {
            issue.Description = &text
        }
    }

    // Priority: string ID → *int
    if ji.Fields.Priority != nil {
        if p, err := strconv.Atoi(ji.Fields.Priority.ID); err == nil {
            issue.Priority = &p
        }
    }

    // URL
    url := baseURL + "/browse/" + ji.Key
    issue.URL = &url

    // BlockedBy: issuelinks where type.inward contains "blocked by"
    for _, link := range ji.Fields.IssueLinks {
        if strings.Contains(strings.ToLower(link.Type.Inward), "blocked by") && link.InwardIssue != nil {
            blocker := Blocker{
                ID:         strPtr(link.InwardIssue.Key),
                Identifier: strPtr(link.InwardIssue.Key),
            }
            if link.InwardIssue.Fields.Status.Name != "" {
                blocker.State = strPtr(link.InwardIssue.Fields.Status.Name)
            }
            issue.BlockedBy = append(issue.BlockedBy, blocker)
        }
    }

    // Timestamps
    issue.CreatedAt = parseTimestamp(ji.Fields.Created)
    issue.UpdatedAt = parseTimestamp(ji.Fields.Updated)

    return issue
}
```

### 6.1 ADF Text Extraction

Jira v3 descriptions use Atlassian Document Format — a nested JSON structure:

```json
{
  "type": "doc",
  "content": [
    {
      "type": "paragraph",
      "content": [
        {"type": "text", "text": "Fix the login bug."},
        {"type": "text", "text": " It happens on Chrome."}
      ]
    }
  ]
}
```

Extractor:

```go
func extractADFText(desc any) string {
    switch v := desc.(type) {
    case string:
        return v  // API v2 fallback
    case map[string]any:
        return extractADFTextFromNode(v)
    default:
        return ""
    }
}

func extractADFTextFromNode(node map[string]any) string {
    var parts []string

    if text, ok := node["text"].(string); ok {
        parts = append(parts, text)
    }

    if content, ok := node["content"].([]any); ok {
        for _, child := range content {
            if childMap, ok := child.(map[string]any); ok {
                parts = append(parts, extractADFTextFromNode(childMap))
            }
        }
    }

    return strings.Join(parts, "")
}
```

---

## 7. Factory Pattern

```go
// internal/tracker/issue.go

func NewTrackerClient(cfg *config.TrackerConfig) (TrackerClient, error) {
    switch cfg.Kind {
    case "linear":
        return NewLinearClient(cfg.Endpoint, cfg.APIKey), nil
    case "jira":
        return NewJiraClient(cfg.Endpoint, cfg.Email, cfg.APIKey), nil
    default:
        return nil, &TrackerError{
            Kind:    ErrUnsupportedTrackerKind,
            Message: fmt.Sprintf("unsupported tracker kind: %q", cfg.Kind),
        }
    }
}
```

---

## 8. Config Resolution Updates

```go
// internal/config/resolve.go — add to ResolveConfig()

// Resolve $VAR in tracker.email
resolved.Tracker.Email = resolveEnvVar(resolved.Tracker.Email)

// Jira-specific env fallbacks
if resolved.Tracker.Kind == "jira" {
    if resolved.Tracker.Email == "" {
        resolved.Tracker.Email = os.Getenv("JIRA_EMAIL")
    }
    if resolved.Tracker.APIKey == "" {
        resolved.Tracker.APIKey = os.Getenv("JIRA_API_TOKEN")
    }
}
```

---

## 9. Error Constants

```go
// internal/tracker/errors.go — add

const (
    ErrJiraAuthFailed     = "jira_auth_failed"
    ErrJiraForbidden      = "jira_forbidden"
    ErrJiraNotFound        = "jira_not_found"
    ErrJiraBadRequest     = "jira_bad_request"
    ErrJiraAPIStatus      = "jira_api_status"
    ErrJiraAPIRequest     = "jira_api_request"
    ErrJiraUnknownPayload = "jira_unknown_payload"
)
```

---

## 10. Backward Compatibility

- `tracker.kind: linear` continues to work exactly as before.
- The new `email` field is ignored when kind is `"linear"`.
- main.go changes from `NewLinearClient(endpoint, apiKey)` to `NewTrackerClient(&cfg.Tracker)` — functionally identical for Linear.
- The `Issue.ID` field stores the Jira key (e.g., `PROJ-123`) instead of a UUID. This works because the orchestrator only uses ID for map lookups and equality checks — never as a UUID.
