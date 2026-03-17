package config

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError holds one or more config validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed: %s", strings.Join(e.Errors, "; "))
}

// ValidateDispatchConfig checks that the config has all required fields
// for the orchestrator to poll and dispatch work.
func ValidateDispatchConfig(cfg *Config) error {
	var errs []string

	// tracker.kind must be present and supported
	if cfg.Tracker.Kind == "" {
		errs = append(errs, "tracker.kind is required")
	} else if cfg.Tracker.Kind != "linear" {
		errs = append(errs, fmt.Sprintf("tracker.kind %q is not supported (only \"linear\")", cfg.Tracker.Kind))
	}

	// tracker.api_key must be non-empty after resolution
	if cfg.Tracker.APIKey == "" {
		errs = append(errs, "tracker.api_key is required (must be non-empty after $VAR resolution)")
	}

	// tracker.project_slug must be present for linear
	if cfg.Tracker.Kind == "linear" && cfg.Tracker.ProjectSlug == "" {
		errs = append(errs, "tracker.project_slug is required when tracker.kind is \"linear\"")
	}

	// gemini.command must be non-empty
	if cfg.Gemini.Command == "" {
		errs = append(errs, "gemini.command is required (must be non-empty)")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
