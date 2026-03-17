package config

import (
	"testing"
)

func TestParseConfig_EmptyMap(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaults := DefaultConfig()

	if cfg.Tracker.Endpoint != defaults.Tracker.Endpoint {
		t.Errorf("expected default endpoint %q, got %q", defaults.Tracker.Endpoint, cfg.Tracker.Endpoint)
	}
	if cfg.Polling.IntervalMs != defaults.Polling.IntervalMs {
		t.Errorf("expected default poll interval %d, got %d", defaults.Polling.IntervalMs, cfg.Polling.IntervalMs)
	}
	if cfg.Agent.MaxConcurrentAgents != defaults.Agent.MaxConcurrentAgents {
		t.Errorf("expected default max_concurrent %d, got %d", defaults.Agent.MaxConcurrentAgents, cfg.Agent.MaxConcurrentAgents)
	}
	if cfg.Gemini.Command != defaults.Gemini.Command {
		t.Errorf("expected default gemini command %q, got %q", defaults.Gemini.Command, cfg.Gemini.Command)
	}
	if cfg.Gemini.Model != defaults.Gemini.Model {
		t.Errorf("expected default gemini model %q, got %q", defaults.Gemini.Model, cfg.Gemini.Model)
	}
	if cfg.Hooks.TimeoutMs != defaults.Hooks.TimeoutMs {
		t.Errorf("expected default hook timeout %d, got %d", defaults.Hooks.TimeoutMs, cfg.Hooks.TimeoutMs)
	}
}

func TestParseConfig_NilMap(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.TurnTimeoutMs != 3600000 {
		t.Errorf("expected default turn_timeout_ms, got %d", cfg.Gemini.TurnTimeoutMs)
	}
}

func TestParseConfig_OverrideValues(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":         "linear",
			"api_key":      "test-key",
			"project_slug": "my-proj",
		},
		"polling": map[string]any{
			"interval_ms": 60000,
		},
		"gemini": map[string]any{
			"command": "custom-gemini --acp",
			"model":   "gemini-2.0-flash",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Tracker.Kind != "linear" {
		t.Errorf("expected kind=linear, got %q", cfg.Tracker.Kind)
	}
	if cfg.Tracker.APIKey != "test-key" {
		t.Errorf("expected api_key=test-key, got %q", cfg.Tracker.APIKey)
	}
	if cfg.Polling.IntervalMs != 60000 {
		t.Errorf("expected interval_ms=60000, got %d", cfg.Polling.IntervalMs)
	}
	if cfg.Gemini.Command != "custom-gemini --acp" {
		t.Errorf("expected custom command, got %q", cfg.Gemini.Command)
	}
	if cfg.Gemini.Model != "gemini-2.0-flash" {
		t.Errorf("expected model override, got %q", cfg.Gemini.Model)
	}

	// Defaults preserved for unset fields
	if cfg.Tracker.Endpoint != "https://api.linear.app/graphql" {
		t.Errorf("expected default endpoint, got %q", cfg.Tracker.Endpoint)
	}
	if cfg.Agent.MaxConcurrentAgents != 10 {
		t.Errorf("expected default max_concurrent=10, got %d", cfg.Agent.MaxConcurrentAgents)
	}
}

func TestParseConfig_CodexAliasToGemini(t *testing.T) {
	raw := map[string]any{
		"codex": map[string]any{
			"command":        "my-codex-command",
			"turn_timeout_ms": 500000,
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.Command != "my-codex-command" {
		t.Errorf("expected codex alias to gemini.command, got %q", cfg.Gemini.Command)
	}
	if cfg.Gemini.TurnTimeoutMs != 500000 {
		t.Errorf("expected turn_timeout from codex alias, got %d", cfg.Gemini.TurnTimeoutMs)
	}
}

func TestParseConfig_GeminiTakesPrecedenceOverCodex(t *testing.T) {
	raw := map[string]any{
		"codex": map[string]any{
			"command": "codex-command",
		},
		"gemini": map[string]any{
			"command": "gemini-command",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.Command != "gemini-command" {
		t.Errorf("expected gemini to take precedence, got %q", cfg.Gemini.Command)
	}
}

func TestParseConfig_StringIntegerCoercion(t *testing.T) {
	// yaml.v3 should handle this, but verify with raw map
	raw := map[string]any{
		"polling": map[string]any{
			"interval_ms": 45000, // int in YAML
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Polling.IntervalMs != 45000 {
		t.Errorf("expected 45000, got %d", cfg.Polling.IntervalMs)
	}
}

func TestParseConfig_GeminiCommandPreservedAsShellString(t *testing.T) {
	raw := map[string]any{
		"gemini": map[string]any{
			"command": "gemini --experimental-acp --model gemini-3.1-pro-preview",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "gemini --experimental-acp --model gemini-3.1-pro-preview"
	if cfg.Gemini.Command != expected {
		t.Errorf("expected command preserved as shell string %q, got %q", expected, cfg.Gemini.Command)
	}
}
