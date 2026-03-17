package tracker

import (
	"strings"
	"time"
)

// linearIssueNode represents the raw issue data from Linear GraphQL responses.
type linearIssueNode struct {
	ID          string  `json:"id"`
	Identifier  string  `json:"identifier"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Priority    any     `json:"priority"`
	State       *struct {
		Name string `json:"name"`
	} `json:"state"`
	BranchName *string `json:"branchName"`
	URL        *string `json:"url"`
	Labels     *struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Relations *struct {
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
	} `json:"relations"`
	CreatedAt *string `json:"createdAt"`
	UpdatedAt *string `json:"updatedAt"`
}

// normalizeIssue converts a raw Linear issue node to the domain Issue model.
func normalizeIssue(raw linearIssueNode) Issue {
	issue := Issue{
		ID:          raw.ID,
		Identifier:  raw.Identifier,
		Title:       raw.Title,
		Description: raw.Description,
		BranchName:  raw.BranchName,
		URL:         raw.URL,
		Labels:      []string{},
		BlockedBy:   []Blocker{},
	}

	// State
	if raw.State != nil {
		issue.State = raw.State.Name
	}

	// Priority: integer only, non-integers become nil
	issue.Priority = normalizePriority(raw.Priority)

	// Labels: normalized to lowercase
	if raw.Labels != nil {
		for _, l := range raw.Labels.Nodes {
			issue.Labels = append(issue.Labels, strings.ToLower(l.Name))
		}
	}

	// BlockedBy: derived from inverse relations where type is "blocks"
	if raw.Relations != nil {
		for _, rel := range raw.Relations.Nodes {
			if rel.Type == "blocks" && rel.RelatedIssue != nil {
				blocker := Blocker{
					ID:         strPtr(rel.RelatedIssue.ID),
					Identifier: strPtr(rel.RelatedIssue.Identifier),
				}
				if rel.RelatedIssue.State != nil {
					blocker.State = strPtr(rel.RelatedIssue.State.Name)
				}
				issue.BlockedBy = append(issue.BlockedBy, blocker)
			}
		}
	}

	// Timestamps
	issue.CreatedAt = parseTimestamp(raw.CreatedAt)
	issue.UpdatedAt = parseTimestamp(raw.UpdatedAt)

	return issue
}

func normalizePriority(v any) *int {
	switch p := v.(type) {
	case float64:
		i := int(p)
		return &i
	case int:
		return &p
	default:
		return nil
	}
}

func parseTimestamp(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05.000Z", *s)
		if err != nil {
			return nil
		}
	}
	return &t
}

func strPtr(s string) *string {
	return &s
}
