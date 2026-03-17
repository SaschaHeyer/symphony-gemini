package prompt

import (
	"github.com/symphony-go/symphony/internal/tracker"
)

// BuildContext constructs the template variable context from an issue and attempt.
func BuildContext(issue *tracker.Issue, attempt *int) map[string]any {
	issueMap := map[string]any{
		"id":          issue.ID,
		"identifier":  issue.Identifier,
		"title":       issue.Title,
		"state":       issue.State,
		"labels":      issue.Labels,
		"blocked_by":  buildBlockers(issue.BlockedBy),
	}

	// Optional fields: nil → not set in template context
	if issue.Description != nil {
		issueMap["description"] = *issue.Description
	} else {
		issueMap["description"] = ""
	}

	if issue.Priority != nil {
		issueMap["priority"] = *issue.Priority
	} else {
		issueMap["priority"] = nil
	}

	if issue.BranchName != nil {
		issueMap["branch_name"] = *issue.BranchName
	} else {
		issueMap["branch_name"] = ""
	}

	if issue.URL != nil {
		issueMap["url"] = *issue.URL
	} else {
		issueMap["url"] = ""
	}

	if issue.CreatedAt != nil {
		issueMap["created_at"] = issue.CreatedAt.Format("2006-01-02T15:04:05Z")
	} else {
		issueMap["created_at"] = ""
	}

	if issue.UpdatedAt != nil {
		issueMap["updated_at"] = issue.UpdatedAt.Format("2006-01-02T15:04:05Z")
	} else {
		issueMap["updated_at"] = ""
	}

	ctx := map[string]any{
		"issue": issueMap,
	}

	if attempt != nil {
		ctx["attempt"] = *attempt
	} else {
		ctx["attempt"] = nil
	}

	return ctx
}

func buildBlockers(blockers []tracker.Blocker) []map[string]any {
	result := make([]map[string]any, len(blockers))
	for i, b := range blockers {
		m := map[string]any{}
		if b.ID != nil {
			m["id"] = *b.ID
		}
		if b.Identifier != nil {
			m["identifier"] = *b.Identifier
		}
		if b.State != nil {
			m["state"] = *b.State
		}
		result[i] = m
	}
	return result
}
