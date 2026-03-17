package prompt

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/tracker"
)

func testIssue() *tracker.Issue {
	desc := "Fix the login bug"
	url := "https://linear.app/team/MT-42"
	priority := 2
	created := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)

	return &tracker.Issue{
		ID:          "issue-42",
		Identifier:  "MT-42",
		Title:       "Fix login bug",
		Description: &desc,
		Priority:    &priority,
		State:       "Todo",
		URL:         &url,
		Labels:      []string{"bug", "urgent"},
		BlockedBy: []tracker.Blocker{
			{
				ID:         strPtr("blocker-1"),
				Identifier: strPtr("MT-41"),
				State:      strPtr("In Progress"),
			},
		},
		CreatedAt: &created,
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestRenderPrompt_BasicFields(t *testing.T) {
	result, err := RenderPrompt("Work on {{ issue.identifier }}: {{ issue.title }}", testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Work on MT-42: Fix login bug" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRenderPrompt_Labels(t *testing.T) {
	tmpl := "Labels: {% for label in issue.labels %}{{ label }} {% endfor %}"
	result, err := RenderPrompt(tmpl, testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "bug") || !strings.Contains(result, "urgent") {
		t.Errorf("expected labels in output, got: %q", result)
	}
}

func TestRenderPrompt_NilDescription(t *testing.T) {
	issue := testIssue()
	issue.Description = nil

	result, err := RenderPrompt("Desc: {{ issue.description }}", issue, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should render as empty string, not error
	if result != "Desc: " {
		t.Errorf("expected empty description, got: %q", result)
	}
}

func TestRenderPrompt_AttemptNilOnFirstRun(t *testing.T) {
	result, err := RenderPrompt("Attempt: {{ attempt }}", testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil renders as empty in Liquid
	if strings.TrimSpace(result) != "Attempt:" {
		t.Errorf("expected empty attempt, got: %q", result)
	}
}

func TestRenderPrompt_AttemptInteger(t *testing.T) {
	attempt := 3
	result, err := RenderPrompt("Attempt: {{ attempt }}", testIssue(), &attempt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Attempt: 3" {
		t.Errorf("expected 'Attempt: 3', got: %q", result)
	}
}

func TestRenderPrompt_EmptyTemplate(t *testing.T) {
	result, err := RenderPrompt("", testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "You are working on an issue from Linear." {
		t.Errorf("expected default prompt, got: %q", result)
	}
}

func TestRenderPrompt_WhitespaceOnlyTemplate(t *testing.T) {
	result, err := RenderPrompt("   \n  \t  ", testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "You are working on an issue from Linear." {
		t.Errorf("expected default prompt, got: %q", result)
	}
}

func TestRenderPrompt_InvalidTemplateSyntax(t *testing.T) {
	// Use an invalid tag that triggers a parse error
	_, err := RenderPrompt("{% for %}", testIssue(), nil)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
	if !errors.Is(err, ErrTemplateParseError) && !errors.Is(err, ErrTemplateRenderError) {
		t.Errorf("expected template error, got: %v", err)
	}
}

func TestRenderPrompt_ComplexTemplate(t *testing.T) {
	tmpl := `Issue: {{ issue.identifier }} - {{ issue.title }}
State: {{ issue.state }}
Priority: {{ issue.priority }}
{% if issue.labels.size > 0 %}Labels: {% for l in issue.labels %}{{ l }}{% unless forloop.last %}, {% endunless %}{% endfor %}{% endif %}`

	result, err := RenderPrompt(tmpl, testIssue(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "MT-42") {
		t.Errorf("expected identifier in output: %q", result)
	}
	if !strings.Contains(result, "Todo") {
		t.Errorf("expected state in output: %q", result)
	}
}
