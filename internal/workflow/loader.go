package workflow

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrMissingWorkflowFile  = errors.New("missing_workflow_file")
	ErrWorkflowParseError   = errors.New("workflow_parse_error")
	ErrFrontMatterNotMap    = errors.New("workflow_front_matter_not_a_map")
)

// WorkflowDefinition represents a parsed WORKFLOW.md file.
type WorkflowDefinition struct {
	Config         map[string]any
	PromptTemplate string
}

// LoadWorkflow reads and parses a WORKFLOW.md file at the given path.
// It splits YAML front matter (between --- delimiters) from the prompt body.
func LoadWorkflow(path string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrMissingWorkflowFile, path)
		}
		return nil, fmt.Errorf("%w: %v", ErrMissingWorkflowFile, err)
	}

	content := string(data)
	config, promptBody, err := splitFrontMatter(content)
	if err != nil {
		return nil, err
	}

	return &WorkflowDefinition{
		Config:         config,
		PromptTemplate: strings.TrimSpace(promptBody),
	}, nil
}

// splitFrontMatter splits content into YAML config map and prompt body.
// If no --- delimiters are found, the entire content is the prompt body.
func splitFrontMatter(content string) (map[string]any, string, error) {
	if !strings.HasPrefix(content, "---") {
		return map[string]any{}, content, nil
	}

	// Split into lines and find the closing ---
	lines := strings.SplitAfter(content, "\n")
	openFound := false
	closeIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "---" {
			if !openFound {
				openFound = true
				continue
			}
			closeIdx = i
			break
		}
	}

	if !openFound || closeIdx == -1 {
		// No proper front matter delimiters; entire content is prompt body
		return map[string]any{}, content, nil
	}

	// YAML content is lines between opening and closing ---
	var yamlLines []string
	for i := 1; i < closeIdx; i++ {
		yamlLines = append(yamlLines, lines[i])
	}
	yamlContent := strings.Join(yamlLines, "")

	// Prompt body is everything after closing ---
	var bodyLines []string
	for i := closeIdx + 1; i < len(lines); i++ {
		bodyLines = append(bodyLines, lines[i])
	}
	promptBody := strings.Join(bodyLines, "")

	// Parse YAML
	if strings.TrimSpace(yamlContent) == "" {
		return map[string]any{}, promptBody, nil
	}

	var parsed any
	if err := yaml.Unmarshal([]byte(yamlContent), &parsed); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrWorkflowParseError, err)
	}

	configMap, ok := parsed.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("%w: front matter must be a YAML map, got %T", ErrFrontMatterNotMap, parsed)
	}

	return configMap, promptBody, nil
}
