package tracker

import (
	"testing"
)

func TestNormalizeJiraIssue_FullFields(t *testing.T) {
	created := "2026-01-15T10:30:00Z"
	updated := "2026-02-20T14:00:00Z"

	ji := jiraIssue{
		Key: "PROJ-123",
		Fields: jiraIssueFields{
			Summary: "Implement feature X",
			Description: map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{
								"type": "text",
								"text": "This is the description.",
							},
						},
					},
				},
			},
			Priority: &jiraPriority{ID: "2", Name: "High"},
			Status:   &jiraStatus{Name: "In Progress"},
			Labels:   []string{"Backend", "URGENT"},
			IssueLinks: []jiraIssueLink{
				{
					Type: jiraIssueLinkType{Inward: "is blocked by", Outward: "blocks"},
					InwardIssue: &jiraLinkedIssue{
						Key: "PROJ-100",
						Fields: &struct {
							Status *jiraStatus `json:"status"`
						}{
							Status: &jiraStatus{Name: "Done"},
						},
					},
				},
			},
			Created: &created,
			Updated: &updated,
		},
	}

	issue := normalizeJiraIssue(ji, "https://mycompany.atlassian.net")

	if issue.ID != "PROJ-123" {
		t.Errorf("expected ID=PROJ-123, got %s", issue.ID)
	}
	if issue.Identifier != "PROJ-123" {
		t.Errorf("expected Identifier=PROJ-123, got %s", issue.Identifier)
	}
	if issue.Title != "Implement feature X" {
		t.Errorf("expected title='Implement feature X', got %s", issue.Title)
	}
	if issue.Description == nil || *issue.Description != "This is the description." {
		t.Errorf("expected description='This is the description.', got %v", issue.Description)
	}
	if issue.Priority == nil || *issue.Priority != 2 {
		t.Errorf("expected priority=2, got %v", issue.Priority)
	}
	if issue.State != "In Progress" {
		t.Errorf("expected state='In Progress', got %s", issue.State)
	}
	if issue.BranchName != nil {
		t.Errorf("expected nil BranchName, got %v", *issue.BranchName)
	}
	if issue.URL == nil || *issue.URL != "https://mycompany.atlassian.net/browse/PROJ-123" {
		t.Errorf("expected URL=https://mycompany.atlassian.net/browse/PROJ-123, got %v", issue.URL)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "backend" || issue.Labels[1] != "urgent" {
		t.Errorf("expected lowercased labels [backend, urgent], got %v", issue.Labels)
	}
	if len(issue.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(issue.BlockedBy))
	}
	if *issue.BlockedBy[0].ID != "PROJ-100" {
		t.Errorf("expected blocker ID=PROJ-100, got %s", *issue.BlockedBy[0].ID)
	}
	if *issue.BlockedBy[0].State != "Done" {
		t.Errorf("expected blocker state=Done, got %s", *issue.BlockedBy[0].State)
	}
	if issue.CreatedAt == nil {
		t.Fatal("expected non-nil CreatedAt")
	}
	if issue.UpdatedAt == nil {
		t.Fatal("expected non-nil UpdatedAt")
	}
}

func TestNormalizeJiraIssue_MinimalFields(t *testing.T) {
	ji := jiraIssue{
		Key: "PROJ-1",
		Fields: jiraIssueFields{
			Summary: "Minimal issue",
			Status:  &jiraStatus{Name: "Open"},
		},
	}

	issue := normalizeJiraIssue(ji, "https://jira.example.com")

	if issue.ID != "PROJ-1" {
		t.Errorf("expected ID=PROJ-1, got %s", issue.ID)
	}
	if issue.Description != nil {
		t.Errorf("expected nil description, got %v", *issue.Description)
	}
	if issue.Priority != nil {
		t.Errorf("expected nil priority, got %v", *issue.Priority)
	}
	if issue.BranchName != nil {
		t.Errorf("expected nil BranchName, got %v", *issue.BranchName)
	}
	if issue.CreatedAt != nil {
		t.Errorf("expected nil CreatedAt")
	}
	if issue.UpdatedAt != nil {
		t.Errorf("expected nil UpdatedAt")
	}
	if len(issue.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", issue.Labels)
	}
	if len(issue.BlockedBy) != 0 {
		t.Errorf("expected empty blocked_by, got %v", issue.BlockedBy)
	}
}

func TestNormalizeJiraIssue_LabelsLowercase(t *testing.T) {
	ji := jiraIssue{
		Key: "PROJ-1",
		Fields: jiraIssueFields{
			Summary: "Test",
			Status:  &jiraStatus{Name: "Open"},
			Labels:  []string{"Bug", "URGENT", "Feature Request"},
		},
	}

	issue := normalizeJiraIssue(ji, "https://jira.example.com")

	expected := []string{"bug", "urgent", "feature request"}
	if len(issue.Labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d", len(expected), len(issue.Labels))
	}
	for i, label := range issue.Labels {
		if label != expected[i] {
			t.Errorf("label[%d]: expected %q, got %q", i, expected[i], label)
		}
	}
}

func TestNormalizeJiraIssue_PriorityParsing(t *testing.T) {
	ji := jiraIssue{
		Key: "PROJ-1",
		Fields: jiraIssueFields{
			Summary:  "Test",
			Status:   &jiraStatus{Name: "Open"},
			Priority: &jiraPriority{ID: "2", Name: "High"},
		},
	}

	issue := normalizeJiraIssue(ji, "https://jira.example.com")
	if issue.Priority == nil {
		t.Fatal("expected non-nil priority")
	}
	if *issue.Priority != 2 {
		t.Errorf("expected priority=2, got %d", *issue.Priority)
	}
}

func TestNormalizeJiraIssue_BlockedBy(t *testing.T) {
	ji := jiraIssue{
		Key: "PROJ-1",
		Fields: jiraIssueFields{
			Summary: "Test",
			Status:  &jiraStatus{Name: "Open"},
			IssueLinks: []jiraIssueLink{
				{
					// Should match: inward contains "blocked by"
					Type: jiraIssueLinkType{Inward: "is blocked by", Outward: "blocks"},
					InwardIssue: &jiraLinkedIssue{
						Key: "PROJ-50",
						Fields: &struct {
							Status *jiraStatus `json:"status"`
						}{
							Status: &jiraStatus{Name: "In Progress"},
						},
					},
				},
				{
					// Should not match: inward does not contain "blocked by"
					Type:        jiraIssueLinkType{Inward: "is cloned by", Outward: "clones"},
					InwardIssue: &jiraLinkedIssue{Key: "PROJ-51"},
				},
				{
					// Should not match: no inward issue present
					Type:         jiraIssueLinkType{Inward: "is blocked by", Outward: "blocks"},
					OutwardIssue: &jiraLinkedIssue{Key: "PROJ-52"},
				},
			},
		},
	}

	issue := normalizeJiraIssue(ji, "https://jira.example.com")

	if len(issue.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(issue.BlockedBy))
	}
	if *issue.BlockedBy[0].ID != "PROJ-50" {
		t.Errorf("expected blocker ID=PROJ-50, got %s", *issue.BlockedBy[0].ID)
	}
	if *issue.BlockedBy[0].State != "In Progress" {
		t.Errorf("expected blocker state='In Progress', got %s", *issue.BlockedBy[0].State)
	}
}

func TestNormalizeJiraIssue_URL(t *testing.T) {
	ji := jiraIssue{
		Key: "PROJ-42",
		Fields: jiraIssueFields{
			Summary: "Test",
			Status:  &jiraStatus{Name: "Open"},
		},
	}

	issue := normalizeJiraIssue(ji, "https://myco.atlassian.net")
	if issue.URL == nil {
		t.Fatal("expected non-nil URL")
	}
	if *issue.URL != "https://myco.atlassian.net/browse/PROJ-42" {
		t.Errorf("expected URL=https://myco.atlassian.net/browse/PROJ-42, got %s", *issue.URL)
	}

	// Test with trailing slash in base URL
	issue2 := normalizeJiraIssue(ji, "https://myco.atlassian.net/")
	if *issue2.URL != "https://myco.atlassian.net/browse/PROJ-42" {
		t.Errorf("trailing slash not trimmed: got %s", *issue2.URL)
	}
}

func TestExtractADFText_Simple(t *testing.T) {
	adf := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "Hello world",
					},
				},
			},
		},
	}

	result := extractADFText(adf)
	if result != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", result)
	}
}

func TestExtractADFText_Nested(t *testing.T) {
	adf := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "First paragraph.",
					},
				},
			},
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "Second ",
					},
					map[string]any{
						"type": "text",
						"text": "paragraph.",
					},
				},
			},
		},
	}

	result := extractADFText(adf)
	expected := "First paragraph.\nSecond paragraph."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractADFText_String(t *testing.T) {
	result := extractADFText("plain text description")
	if result != "plain text description" {
		t.Errorf("expected 'plain text description', got %q", result)
	}
}

func TestExtractADFText_Nil(t *testing.T) {
	result := extractADFText(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildJQL_Basic(t *testing.T) {
	jql := buildJQL("PROJ", []string{"To Do", "In Progress"})
	expected := `project = "PROJ" AND status IN ("To Do", "In Progress") ORDER BY created ASC`
	if jql != expected {
		t.Errorf("expected %q, got %q", expected, jql)
	}
}

func TestBuildJQL_QuoteEscaping(t *testing.T) {
	jql := buildJQL("MY-PROJECT", []string{`Won't Fix`, `Status "A"`})
	// Quotes inside values should be escaped
	if jql == "" {
		t.Fatal("expected non-empty JQL")
	}
	// The value with a double quote should be escaped
	expected := `project = "MY-PROJECT" AND status IN ("Won't Fix", "Status \"A\"") ORDER BY created ASC`
	if jql != expected {
		t.Errorf("expected %q, got %q", expected, jql)
	}
}
