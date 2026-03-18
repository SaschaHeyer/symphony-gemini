package tracker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJiraFetchCandidateIssues_CorrectJQL(t *testing.T) {
	var receivedJQL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedJQL = r.URL.Query().Get("jql")

		json.NewEncoder(w).Encode(jiraSearchResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      1,
			Issues: []jiraIssue{
				{
					Key: "PROJ-1",
					Fields: jiraIssueFields{
						Summary: "Test issue",
						Status:  &jiraStatus{Name: "To Do"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchCandidateIssues("PROJ", []string{"To Do", "In Progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(receivedJQL, `project = "PROJ"`) {
		t.Errorf("expected JQL to contain project = \"PROJ\", got %q", receivedJQL)
	}
	if !strings.Contains(receivedJQL, `"To Do"`) {
		t.Errorf("expected JQL to contain \"To Do\", got %q", receivedJQL)
	}
	if !strings.Contains(receivedJQL, `"In Progress"`) {
		t.Errorf("expected JQL to contain \"In Progress\", got %q", receivedJQL)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Identifier != "PROJ-1" {
		t.Errorf("expected PROJ-1, got %s", issues[0].Identifier)
	}
}

func TestJiraFetchCandidateIssues_BasicAuth(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		json.NewEncoder(w).Encode(jiraSearchResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      0,
			Issues:     []jiraIssue{},
		})
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	_, err := client.FetchCandidateIssues("PROJ", []string{"To Do"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(receivedAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", receivedAuth)
	}

	// Verify the base64 encoded credentials
	if !strings.Contains(receivedAuth, "Basic ") {
		t.Errorf("expected Basic auth, got %q", receivedAuth)
	}
}

func TestJiraFetchCandidateIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			json.NewEncoder(w).Encode(jiraSearchResponse{
				StartAt:    0,
				MaxResults: 1,
				Total:      2,
				Issues: []jiraIssue{
					{
						Key: "PROJ-1",
						Fields: jiraIssueFields{
							Summary: "First",
							Status:  &jiraStatus{Name: "To Do"},
						},
					},
				},
			})
		} else {
			startAt := r.URL.Query().Get("startAt")
			if startAt != "1" {
				t.Errorf("expected startAt=1, got %s", startAt)
			}

			json.NewEncoder(w).Encode(jiraSearchResponse{
				StartAt:    1,
				MaxResults: 1,
				Total:      2,
				Issues: []jiraIssue{
					{
						Key: "PROJ-2",
						Fields: jiraIssueFields{
							Summary: "Second",
							Status:  &jiraStatus{Name: "To Do"},
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchCandidateIssues("PROJ", []string{"To Do"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues across pages, got %d", len(issues))
	}
	if issues[0].Identifier != "PROJ-1" || issues[1].Identifier != "PROJ-2" {
		t.Errorf("unexpected order: %s, %s", issues[0].Identifier, issues[1].Identifier)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestJiraFetchCandidateIssues_EmptyStates(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchCandidateIssues("PROJ", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty, got %d issues", len(issues))
	}
	if callCount != 0 {
		t.Errorf("expected no HTTP calls, got %d", callCount)
	}
}

func TestJiraFetchIssueStatesByIDs_CorrectJQL(t *testing.T) {
	var receivedJQL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedJQL = r.URL.Query().Get("jql")

		json.NewEncoder(w).Encode(jiraSearchResponse{
			StartAt:    0,
			MaxResults: 50,
			Total:      1,
			Issues: []jiraIssue{
				{
					Key: "PROJ-1",
					Fields: jiraIssueFields{
						Summary: "Test",
						Status:  &jiraStatus{Name: "Done"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchIssueStatesByIDs([]string{"PROJ-1", "PROJ-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(receivedJQL, "key IN") {
		t.Errorf("expected JQL to contain 'key IN', got %q", receivedJQL)
	}
	if !strings.Contains(receivedJQL, `"PROJ-1"`) {
		t.Errorf("expected JQL to contain PROJ-1, got %q", receivedJQL)
	}
	if !strings.Contains(receivedJQL, `"PROJ-2"`) {
		t.Errorf("expected JQL to contain PROJ-2, got %q", receivedJQL)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].State != "Done" {
		t.Errorf("expected state=Done, got %s", issues[0].State)
	}
}

func TestJiraFetchIssueStatesByIDs_EmptyIDs(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchIssueStatesByIDs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty, got %d issues", len(issues))
	}
	if callCount != 0 {
		t.Errorf("expected no HTTP calls, got %d", callCount)
	}
}

func TestJiraFetchIssuesByStates_EmptyStates(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	issues, err := client.FetchIssuesByStates("PROJ", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty, got %d", len(issues))
	}
	if callCount != 0 {
		t.Errorf("expected no HTTP calls, got %d", callCount)
	}
}

func TestJiraClient_HTTP401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "bad-token")
	_, err := client.FetchCandidateIssues("PROJ", []string{"To Do"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrJiraAuthFailed {
		t.Errorf("expected %s, got %s", ErrJiraAuthFailed, te.Kind)
	}
}

func TestJiraClient_HTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	_, err := client.FetchCandidateIssues("PROJ", []string{"To Do"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrJiraBadRequest {
		t.Errorf("expected %s, got %s", ErrJiraBadRequest, te.Kind)
	}
}

func TestJiraClient_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	client := NewJiraClient(server.URL, "user@example.com", "token123")
	_, err := client.FetchCandidateIssues("PROJ", []string{"To Do"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrJiraUnknownPayload {
		t.Errorf("expected %s, got %s", ErrJiraUnknownPayload, te.Kind)
	}
}
