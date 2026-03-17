package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultPageSize    = 50
	defaultNetTimeout  = 30 * time.Second
)

// LinearClient implements TrackerClient for Linear's GraphQL API.
type LinearClient struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewLinearClient creates a new Linear GraphQL client.
func NewLinearClient(endpoint, apiKey string) *LinearClient {
	return &LinearClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: defaultNetTimeout,
		},
	}
}

// graphqlRequest is the shape of a GraphQL HTTP POST body.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlResponse is the top-level GraphQL response shape.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// issuesResponse wraps the "issues" field from GraphQL data.
type issuesResponse struct {
	Issues struct {
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []linearIssueNode `json:"nodes"`
	} `json:"issues"`
}

// FetchCandidateIssues fetches issues in active states for a project, with pagination.
func (c *LinearClient) FetchCandidateIssues(projectSlug string, activeStates []string) ([]Issue, error) {
	var allIssues []Issue
	var cursor *string

	for {
		vars := map[string]any{
			"projectSlug": projectSlug,
			"states":      activeStates,
		}
		if cursor != nil {
			vars["after"] = *cursor
		}

		data, err := c.execute(candidateIssuesQuery, vars)
		if err != nil {
			return nil, err
		}

		var resp issuesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, newTrackerError(ErrLinearUnknownPayload, "failed to parse issues response", err)
		}

		for _, node := range resp.Issues.Nodes {
			allIssues = append(allIssues, normalizeIssue(node))
		}

		if !resp.Issues.PageInfo.HasNextPage {
			break
		}

		if resp.Issues.PageInfo.EndCursor == nil {
			return nil, newTrackerError(ErrLinearMissingEndCursor, "hasNextPage=true but endCursor is nil", nil)
		}
		cursor = resp.Issues.PageInfo.EndCursor
	}

	return allIssues, nil
}

// FetchIssueStatesByIDs fetches current states for specific issue IDs.
func (c *LinearClient) FetchIssueStatesByIDs(ids []string) ([]Issue, error) {
	if len(ids) == 0 {
		return []Issue{}, nil
	}

	vars := map[string]any{
		"ids": ids,
	}

	data, err := c.execute(issueStatesByIDsQuery, vars)
	if err != nil {
		return nil, err
	}

	var resp issuesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, newTrackerError(ErrLinearUnknownPayload, "failed to parse state refresh response", err)
	}

	var issues []Issue
	for _, node := range resp.Issues.Nodes {
		issues = append(issues, normalizeIssue(node))
	}

	return issues, nil
}

// FetchIssuesByStates fetches issues in specific states for a project, with pagination.
func (c *LinearClient) FetchIssuesByStates(projectSlug string, states []string) ([]Issue, error) {
	if len(states) == 0 {
		return []Issue{}, nil
	}

	var allIssues []Issue
	var cursor *string

	for {
		vars := map[string]any{
			"projectSlug": projectSlug,
			"states":      states,
		}
		if cursor != nil {
			vars["after"] = *cursor
		}

		data, err := c.execute(issuesByStatesQuery, vars)
		if err != nil {
			return nil, err
		}

		var resp issuesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, newTrackerError(ErrLinearUnknownPayload, "failed to parse issues by states response", err)
		}

		for _, node := range resp.Issues.Nodes {
			allIssues = append(allIssues, normalizeIssue(node))
		}

		if !resp.Issues.PageInfo.HasNextPage {
			break
		}

		if resp.Issues.PageInfo.EndCursor == nil {
			return nil, newTrackerError(ErrLinearMissingEndCursor, "hasNextPage=true but endCursor is nil", nil)
		}
		cursor = resp.Issues.PageInfo.EndCursor
	}

	return allIssues, nil
}

// execute performs a GraphQL request and returns the "data" field.
func (c *LinearClient) execute(query string, variables map[string]any) (json.RawMessage, error) {
	reqBody := graphqlRequest{
		Query:     query,
		Variables: variables,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, newTrackerError(ErrLinearAPIRequest, "failed to marshal request", err)
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, newTrackerError(ErrLinearAPIRequest, "failed to create request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, newTrackerError(ErrLinearAPIRequest, "request failed", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newTrackerError(ErrLinearAPIRequest, "failed to read response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, newTrackerError(ErrLinearAPIStatus,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)), nil)
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, newTrackerError(ErrLinearUnknownPayload, "failed to parse GraphQL response", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return nil, newTrackerError(ErrLinearGraphQLErrors,
			fmt.Sprintf("GraphQL errors: %v", msgs), nil)
	}

	if gqlResp.Data == nil {
		return nil, newTrackerError(ErrLinearUnknownPayload, "response missing data field", nil)
	}

	return gqlResp.Data, nil
}
