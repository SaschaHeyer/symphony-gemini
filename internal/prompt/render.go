package prompt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/osteele/liquid"
	"github.com/symphony-go/symphony/internal/tracker"
)

var (
	ErrTemplateParseError  = errors.New("template_parse_error")
	ErrTemplateRenderError = errors.New("template_render_error")
)

const defaultPrompt = "You are working on an issue from Linear."

// RenderPrompt renders a workflow prompt template with issue data.
// Uses Liquid-compatible strict template rendering.
func RenderPrompt(templateStr string, issue *tracker.Issue, attempt *int) (string, error) {
	trimmed := strings.TrimSpace(templateStr)
	if trimmed == "" {
		return defaultPrompt, nil
	}

	engine := liquid.NewEngine()

	tpl, err := engine.ParseString(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateParseError, err)
	}

	ctx := BuildContext(issue, attempt)

	result, err := tpl.RenderString(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderError, err)
	}

	return result, nil
}
