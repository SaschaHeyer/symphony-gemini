package tracker

import (
	"fmt"
	"strconv"
	"strings"
)

// normalizeJiraIssue converts a raw Jira issue to the domain Issue model.
func normalizeJiraIssue(ji jiraIssue, baseURL string) Issue {
	issue := Issue{
		ID:         ji.Key,
		Identifier: ji.Key,
		Title:      ji.Fields.Summary,
		Labels:     []string{},
		BlockedBy:  []Blocker{},
	}

	// Description: extract plain text from ADF or string
	desc := extractADFText(ji.Fields.Description)
	if desc != "" {
		issue.Description = &desc
	}

	// Priority: parse ID to int
	if ji.Fields.Priority != nil && ji.Fields.Priority.ID != "" {
		if p, err := strconv.Atoi(ji.Fields.Priority.ID); err == nil {
			issue.Priority = &p
		}
	}

	// State
	if ji.Fields.Status != nil {
		issue.State = ji.Fields.Status.Name
	}

	// BranchName: not native in Jira
	issue.BranchName = nil

	// URL
	url := fmt.Sprintf("%s/browse/%s", strings.TrimRight(baseURL, "/"), ji.Key)
	issue.URL = &url

	// Labels: lowercased
	issue.Labels = lowercaseLabels(ji.Fields.Labels)

	// BlockedBy: issue links where type.inward contains "blocked by" (case-insensitive)
	for _, link := range ji.Fields.IssueLinks {
		if strings.Contains(strings.ToLower(link.Type.Inward), "blocked by") && link.InwardIssue != nil {
			blocker := Blocker{
				ID:         strPtr(link.InwardIssue.Key),
				Identifier: strPtr(link.InwardIssue.Key),
			}
			if link.InwardIssue.Fields != nil && link.InwardIssue.Fields.Status != nil {
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

// extractADFText extracts plain text from an ADF document or returns a string as-is.
func extractADFText(desc any) string {
	if desc == nil {
		return ""
	}

	// Plain string fallback (Jira v2 API)
	if s, ok := desc.(string); ok {
		return s
	}

	// ADF document (map)
	if doc, ok := desc.(map[string]any); ok {
		return extractADFTextFromNode(doc)
	}

	return ""
}

// extractADFTextFromNode recursively extracts text from an ADF node.
func extractADFTextFromNode(node map[string]any) string {
	// If this is a text node, return its text
	if nodeType, ok := node["type"].(string); ok && nodeType == "text" {
		if text, ok := node["text"].(string); ok {
			return text
		}
		return ""
	}

	// Recurse into content array
	content, ok := node["content"].([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, child := range content {
		if childMap, ok := child.(map[string]any); ok {
			text := extractADFTextFromNode(childMap)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}

	// Join paragraph-level nodes with newlines, inline nodes with nothing
	nodeType, _ := node["type"].(string)
	if nodeType == "doc" {
		return strings.Join(parts, "\n")
	}

	return strings.Join(parts, "")
}

// lowercaseLabels returns a new slice with all labels lowercased.
func lowercaseLabels(labels []string) []string {
	if len(labels) == 0 {
		return []string{}
	}
	result := make([]string, len(labels))
	for i, l := range labels {
		result[i] = strings.ToLower(l)
	}
	return result
}
