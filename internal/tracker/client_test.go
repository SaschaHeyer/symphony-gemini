package tracker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchCandidateIssues_CorrectQuery(t *testing.T) {
	var receivedBody graphqlRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "test-key" {
			t.Errorf("expected test-key, got %q", auth)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false},
					"nodes": []map[string]any{
						{
							"id":         "issue-1",
							"identifier": "MT-1",
							"title":      "Test issue",
							"state":      map[string]any{"name": "Todo"},
							"priority":   1,
							"createdAt":  "2026-01-01T00:00:00Z",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	issues, err := client.FetchCandidateIssues("my-project", []string{"Todo", "In Progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify query uses slugId filter
	if !strings.Contains(receivedBody.Query, "slugId") {
		t.Error("expected query to use slugId filter")
	}

	// Verify variables
	if receivedBody.Variables["projectSlug"] != "my-project" {
		t.Errorf("expected projectSlug=my-project, got %v", receivedBody.Variables["projectSlug"])
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Identifier != "MT-1" {
		t.Errorf("expected MT-1, got %s", issues[0].Identifier)
	}
}

func TestFetchIssueStatesByIDs_UsesIDType(t *testing.T) {
	var receivedBody graphqlRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []map[string]any{
						{"id": "id-1", "identifier": "MT-1", "state": map[string]any{"name": "Done"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	issues, err := client.FetchIssueStatesByIDs([]string{"id-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify query uses [ID!] type
	if !strings.Contains(receivedBody.Query, "[ID!]") {
		t.Error("expected query to use [ID!] variable type")
	}

	if len(issues) != 1 || issues[0].State != "Done" {
		t.Errorf("unexpected issues: %+v", issues)
	}
}

func TestFetchIssueStatesByIDs_EmptyList(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
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

func TestFetchIssuesByStates_EmptyStates(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	issues, err := client.FetchIssuesByStates("proj", []string{})
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

func TestFetchCandidateIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body graphqlRequest
		json.NewDecoder(r.Body).Decode(&body)

		if callCount == 1 {
			// First page
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"pageInfo": map[string]any{
							"hasNextPage": true,
							"endCursor":   "cursor-1",
						},
						"nodes": []map[string]any{
							{"id": "1", "identifier": "MT-1", "title": "First", "state": map[string]any{"name": "Todo"}},
						},
					},
				},
			})
		} else {
			// Second page
			if body.Variables["after"] != "cursor-1" {
				t.Errorf("expected after=cursor-1, got %v", body.Variables["after"])
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"pageInfo": map[string]any{"hasNextPage": false},
						"nodes": []map[string]any{
							{"id": "2", "identifier": "MT-2", "title": "Second", "state": map[string]any{"name": "Todo"}},
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	issues, err := client.FetchCandidateIssues("proj", []string{"Todo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues across pages, got %d", len(issues))
	}
	if issues[0].Identifier != "MT-1" || issues[1].Identifier != "MT-2" {
		t.Errorf("unexpected order: %s, %s", issues[0].Identifier, issues[1].Identifier)
	}
}

func TestFetchCandidateIssues_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	_, err := client.FetchCandidateIssues("proj", []string{"Todo"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrLinearAPIStatus {
		t.Errorf("expected %s, got %s", ErrLinearAPIStatus, te.Kind)
	}
}

func TestFetchCandidateIssues_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{
				{"message": "Variable $projectSlug is not defined"},
			},
		})
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	_, err := client.FetchCandidateIssues("proj", []string{"Todo"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrLinearGraphQLErrors {
		t.Errorf("expected %s, got %s", ErrLinearGraphQLErrors, te.Kind)
	}
}

func TestFetchCandidateIssues_MissingEndCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"pageInfo": map[string]any{
						"hasNextPage": true,
						"endCursor":   nil,
					},
					"nodes": []map[string]any{
						{"id": "1", "identifier": "MT-1", "title": "Test", "state": map[string]any{"name": "Todo"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewLinearClient(server.URL, "test-key")
	_, err := client.FetchCandidateIssues("proj", []string{"Todo"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrLinearMissingEndCursor {
		t.Errorf("expected %s, got %s", ErrLinearMissingEndCursor, te.Kind)
	}
}

func TestFetchCandidateIssues_ConnectionRefused(t *testing.T) {
	client := NewLinearClient("http://127.0.0.1:1", "test-key")
	_, err := client.FetchCandidateIssues("proj", []string{"Todo"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrLinearAPIRequest {
		t.Errorf("expected %s, got %s", ErrLinearAPIRequest, te.Kind)
	}
}
