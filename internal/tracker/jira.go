package tracker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	jiraDefaultPageSize   = 50
	jiraSearchFields      = "summary,description,priority,status,labels,issuelinks,created,updated"
)

// JiraClient implements TrackerClient for Jira's REST API v3.
type JiraClient struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
}

// NewJiraClient creates a new Jira REST API client.
func NewJiraClient(baseURL, email, apiToken string) *JiraClient {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
	return &JiraClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Basic " + creds,
		httpClient: &http.Client{
			Timeout: defaultNetTimeout,
		},
	}
}

// jiraSearchResponse is the top-level Jira search response.
type jiraSearchResponse struct {
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
	Issues     []jiraIssue `json:"issues"`
}

// jiraIssue is a single issue from the Jira search response.
type jiraIssue struct {
	Key    string          `json:"key"`
	Fields jiraIssueFields `json:"fields"`
}

// jiraIssueFields contains the fields of a Jira issue.
type jiraIssueFields struct {
	Summary     string          `json:"summary"`
	Description any             `json:"description"`
	Priority    *jiraPriority   `json:"priority"`
	Status      *jiraStatus     `json:"status"`
	Labels      []string        `json:"labels"`
	IssueLinks  []jiraIssueLink `json:"issuelinks"`
	Created     *string         `json:"created"`
	Updated     *string         `json:"updated"`
}

// jiraPriority represents a Jira priority.
type jiraPriority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// jiraStatus represents a Jira status.
type jiraStatus struct {
	Name string `json:"name"`
}

// jiraIssueLink represents a link between Jira issues.
type jiraIssueLink struct {
	Type         jiraIssueLinkType `json:"type"`
	InwardIssue  *jiraLinkedIssue  `json:"inwardIssue"`
	OutwardIssue *jiraLinkedIssue  `json:"outwardIssue"`
}

// jiraIssueLinkType describes the type of link.
type jiraIssueLinkType struct {
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// jiraLinkedIssue is a linked issue reference.
type jiraLinkedIssue struct {
	Key    string      `json:"key"`
	Fields *struct {
		Status *jiraStatus `json:"status"`
	} `json:"fields"`
}

// FetchCandidateIssues fetches issues in active states for a project, with pagination.
func (c *JiraClient) FetchCandidateIssues(projectSlug string, activeStates []string) ([]Issue, error) {
	if len(activeStates) == 0 {
		return []Issue{}, nil
	}

	jql := buildJQL(projectSlug, activeStates)
	return c.fetchAllPages(jql)
}

// FetchIssueStatesByIDs fetches current states for specific issue keys.
func (c *JiraClient) FetchIssueStatesByIDs(ids []string) ([]Issue, error) {
	if len(ids) == 0 {
		return []Issue{}, nil
	}

	jql := buildKeyJQL(ids)
	return c.fetchAllPages(jql)
}

// FetchIssuesByStates fetches issues in specific states for a project, with pagination.
func (c *JiraClient) FetchIssuesByStates(projectSlug string, states []string) ([]Issue, error) {
	if len(states) == 0 {
		return []Issue{}, nil
	}

	jql := buildJQL(projectSlug, states)
	return c.fetchAllPages(jql)
}

// fetchAllPages fetches all pages from a JQL search.
func (c *JiraClient) fetchAllPages(jql string) ([]Issue, error) {
	var allIssues []Issue
	startAt := 0

	for {
		resp, err := c.search(jql, jiraSearchFields, startAt, jiraDefaultPageSize)
		if err != nil {
			return nil, err
		}

		for _, ji := range resp.Issues {
			allIssues = append(allIssues, normalizeJiraIssue(ji, c.baseURL))
		}

		if startAt+len(resp.Issues) >= resp.Total {
			break
		}
		startAt += len(resp.Issues)
	}

	return allIssues, nil
}

// search executes a GET to /rest/api/3/search with the given parameters.
func (c *JiraClient) search(jql, fields string, startAt, maxResults int) (*jiraSearchResponse, error) {
	url := fmt.Sprintf("%s/rest/api/3/search?jql=%s&fields=%s&startAt=%d&maxResults=%d",
		c.baseURL,
		encodeQueryParam(jql),
		encodeQueryParam(fields),
		startAt,
		maxResults,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, newTrackerError(ErrJiraAPIRequest, "failed to create request", err)
	}

	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, newTrackerError(ErrJiraAPIRequest, "request failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newTrackerError(ErrJiraAPIRequest, "failed to read response", err)
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, newTrackerError(ErrJiraAuthFailed, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		case http.StatusForbidden:
			return nil, newTrackerError(ErrJiraForbidden, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		case http.StatusNotFound:
			return nil, newTrackerError(ErrJiraNotFound, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		case http.StatusBadRequest:
			return nil, newTrackerError(ErrJiraBadRequest, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		default:
			return nil, newTrackerError(ErrJiraAPIStatus, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		}
	}

	var searchResp jiraSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, newTrackerError(ErrJiraUnknownPayload, "failed to parse search response", err)
	}

	return &searchResp, nil
}

// buildJQL builds a JQL query for a project with specific statuses.
func buildJQL(projectSlug string, states []string) string {
	return fmt.Sprintf("project = \"%s\" AND status IN (%s) ORDER BY created ASC",
		escapeJQLString(projectSlug),
		joinQuoted(states),
	)
}

// buildKeyJQL builds a JQL query for specific issue keys.
func buildKeyJQL(keys []string) string {
	return fmt.Sprintf("key IN (%s)", joinQuoted(keys))
}

// joinQuoted joins string values with commas, each value double-quoted.
func joinQuoted(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("\"%s\"", escapeJQLString(v))
	}
	return strings.Join(quoted, ", ")
}

// escapeJQLString escapes double quotes inside JQL string values.
func escapeJQLString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// encodeQueryParam performs URL encoding for query parameter values.
func encodeQueryParam(s string) string {
	// Use net/url-style encoding inline to avoid an import cycle
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteRune(c)
		default:
			encoded := fmt.Sprintf("%c", c)
			for _, byt := range []byte(encoded) {
				fmt.Fprintf(&b, "%%%02X", byt)
			}
		}
	}
	return b.String()
}
