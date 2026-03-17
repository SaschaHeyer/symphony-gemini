package logging

import (
	"log/slog"
	"os"
)

// Setup configures the default slog logger with JSON output to stderr.
func Setup() {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))
}

// WithIssue returns a logger with issue context fields.
func WithIssue(logger *slog.Logger, issueID, issueIdentifier string) *slog.Logger {
	return logger.With(
		slog.String("issue_id", issueID),
		slog.String("issue_identifier", issueIdentifier),
	)
}

// WithSession returns a logger with session context field.
func WithSession(logger *slog.Logger, sessionID string) *slog.Logger {
	return logger.With(slog.String("session_id", sessionID))
}
