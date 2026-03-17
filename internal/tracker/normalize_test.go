package tracker

import (
	"testing"
)

func TestNormalizeIssue_LabelsLowercased(t *testing.T) {
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
		Labels: &struct {
			Nodes []struct{ Name string `json:"name"` } `json:"nodes"`
		}{
			Nodes: []struct{ Name string `json:"name"` }{
				{Name: "Bug"},
				{Name: "URGENT"},
				{Name: "Feature Request"},
			},
		},
	}

	issue := normalizeIssue(raw)

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

func TestNormalizeIssue_BlockersFromInverseBlocks(t *testing.T) {
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
		Relations: &struct {
			Nodes []struct {
				Type         string `json:"type"`
				RelatedIssue *struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
					State      *struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"relatedIssue"`
			} `json:"nodes"`
		}{
			Nodes: []struct {
				Type         string `json:"type"`
				RelatedIssue *struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
					State      *struct {
						Name string `json:"name"`
					} `json:"state"`
				} `json:"relatedIssue"`
			}{
				{
					Type: "blocks",
					RelatedIssue: &struct {
						ID         string `json:"id"`
						Identifier string `json:"identifier"`
						State      *struct {
							Name string `json:"name"`
						} `json:"state"`
					}{
						ID:         "blocker-1",
						Identifier: "MT-2",
						State:      &struct{ Name string `json:"name"` }{Name: "In Progress"},
					},
				},
				{
					Type: "related", // not "blocks" — should be ignored
					RelatedIssue: &struct {
						ID         string `json:"id"`
						Identifier string `json:"identifier"`
						State      *struct {
							Name string `json:"name"`
						} `json:"state"`
					}{
						ID:         "related-1",
						Identifier: "MT-3",
					},
				},
			},
		},
	}

	issue := normalizeIssue(raw)

	if len(issue.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(issue.BlockedBy))
	}
	if *issue.BlockedBy[0].ID != "blocker-1" {
		t.Errorf("expected blocker ID=blocker-1, got %s", *issue.BlockedBy[0].ID)
	}
	if *issue.BlockedBy[0].State != "In Progress" {
		t.Errorf("expected blocker state=In Progress, got %s", *issue.BlockedBy[0].State)
	}
}

func TestNormalizeIssue_PriorityNonIntegerBecomesNil(t *testing.T) {
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
		Priority:   "high", // non-integer
	}

	issue := normalizeIssue(raw)
	if issue.Priority != nil {
		t.Errorf("expected nil priority for non-integer, got %v", *issue.Priority)
	}
}

func TestNormalizeIssue_PriorityInteger(t *testing.T) {
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
		Priority:   float64(2), // JSON numbers are float64
	}

	issue := normalizeIssue(raw)
	if issue.Priority == nil {
		t.Fatal("expected non-nil priority")
	}
	if *issue.Priority != 2 {
		t.Errorf("expected priority=2, got %d", *issue.Priority)
	}
}

func TestNormalizeIssue_TimestampsParsed(t *testing.T) {
	ts := "2026-03-17T10:30:00Z"
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
		CreatedAt:  &ts,
	}

	issue := normalizeIssue(raw)
	if issue.CreatedAt == nil {
		t.Fatal("expected non-nil CreatedAt")
	}
	if issue.CreatedAt.Year() != 2026 || issue.CreatedAt.Month() != 3 || issue.CreatedAt.Day() != 17 {
		t.Errorf("unexpected parsed time: %v", issue.CreatedAt)
	}
}

func TestNormalizeIssue_MissingOptionalFields(t *testing.T) {
	raw := linearIssueNode{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Test",
		State:      &struct{ Name string `json:"name"` }{Name: "Todo"},
	}

	issue := normalizeIssue(raw)

	if issue.Description != nil {
		t.Errorf("expected nil description, got %v", *issue.Description)
	}
	if issue.BranchName != nil {
		t.Errorf("expected nil branch_name, got %v", *issue.BranchName)
	}
	if issue.URL != nil {
		t.Errorf("expected nil url, got %v", *issue.URL)
	}
	if issue.Priority != nil {
		t.Errorf("expected nil priority, got %v", *issue.Priority)
	}
	if issue.CreatedAt != nil {
		t.Errorf("expected nil created_at")
	}
	if len(issue.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", issue.Labels)
	}
	if len(issue.BlockedBy) != 0 {
		t.Errorf("expected empty blocked_by, got %v", issue.BlockedBy)
	}
}
