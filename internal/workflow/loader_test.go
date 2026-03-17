package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflow_ValidFrontMatterAndBody(t *testing.T) {
	path := writeTemp(t, `---
tracker:
  kind: linear
  project_slug: my-project
---
You are working on {{ issue.identifier }}.
`)

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.Config["tracker"] == nil {
		t.Fatal("expected tracker config")
	}
	trackerMap := wf.Config["tracker"].(map[string]any)
	if trackerMap["kind"] != "linear" {
		t.Errorf("expected tracker.kind=linear, got %v", trackerMap["kind"])
	}

	expected := "You are working on {{ issue.identifier }}."
	if wf.PromptTemplate != expected {
		t.Errorf("expected prompt %q, got %q", expected, wf.PromptTemplate)
	}
}

func TestLoadWorkflow_EmptyFrontMatter(t *testing.T) {
	path := writeTemp(t, `---
---
Just the prompt body.
`)

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wf.Config) != 0 {
		t.Errorf("expected empty config, got %v", wf.Config)
	}
	if wf.PromptTemplate != "Just the prompt body." {
		t.Errorf("unexpected prompt: %q", wf.PromptTemplate)
	}
}

func TestLoadWorkflow_NoDelimiters(t *testing.T) {
	path := writeTemp(t, `This is just a prompt with no front matter.`)

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wf.Config) != 0 {
		t.Errorf("expected empty config, got %v", wf.Config)
	}
	if wf.PromptTemplate != "This is just a prompt with no front matter." {
		t.Errorf("unexpected prompt: %q", wf.PromptTemplate)
	}
}

func TestLoadWorkflow_InvalidYAML(t *testing.T) {
	path := writeTemp(t, `---
: invalid: yaml: [
---
Body.
`)

	_, err := LoadWorkflow(path)
	if !errors.Is(err, ErrWorkflowParseError) {
		t.Errorf("expected ErrWorkflowParseError, got %v", err)
	}
}

func TestLoadWorkflow_NonMapYAML(t *testing.T) {
	path := writeTemp(t, `---
- item1
- item2
---
Body.
`)

	_, err := LoadWorkflow(path)
	if !errors.Is(err, ErrFrontMatterNotMap) {
		t.Errorf("expected ErrFrontMatterNotMap, got %v", err)
	}
}

func TestLoadWorkflow_MissingFile(t *testing.T) {
	_, err := LoadWorkflow("/nonexistent/path/WORKFLOW.md")
	if !errors.Is(err, ErrMissingWorkflowFile) {
		t.Errorf("expected ErrMissingWorkflowFile, got %v", err)
	}
}

func TestLoadWorkflow_PromptTrimmed(t *testing.T) {
	path := writeTemp(t, `---
tracker:
  kind: linear
---

  Some prompt with whitespace.

`)

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.PromptTemplate != "Some prompt with whitespace." {
		t.Errorf("expected trimmed prompt, got %q", wf.PromptTemplate)
	}
}

func TestLoadWorkflow_UnknownKeysPreserved(t *testing.T) {
	path := writeTemp(t, `---
tracker:
  kind: linear
custom_extension:
  foo: bar
---
Prompt.
`)

	wf, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ext, ok := wf.Config["custom_extension"]
	if !ok {
		t.Fatal("expected custom_extension to be preserved")
	}
	extMap := ext.(map[string]any)
	if extMap["foo"] != "bar" {
		t.Errorf("expected custom_extension.foo=bar, got %v", extMap["foo"])
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
