package taskwarrior

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CommandError contains safe subprocess metadata for UI and logs.
type CommandError struct {
	Kind     string
	Args     []string
	ExitCode int
	Stderr   string
	Duration time.Duration
	Cause    error
}

func (e *CommandError) Error() string {
	message := strings.TrimSpace(e.Stderr)
	if message == "" && e.Cause != nil {
		message = e.Cause.Error()
	}
	if message == "" {
		message = "command failed"
	}
	if e.ExitCode >= 0 {
		return fmt.Sprintf("%s failed (exit %d): %s", e.Kind, e.ExitCode, message)
	}
	return fmt.Sprintf("%s failed: %s", e.Kind, message)
}

func (e *CommandError) Unwrap() error { return e.Cause }

// Redact removes common credentials and sync secrets from diagnostic text.
// Task descriptions are not added to command errors by the adapter.
func Redact(value string) string {
	value = secretAssignmentRE.ReplaceAllString(value, "$1[REDACTED]")
	value = urlCredentialRE.ReplaceAllString(value, "$1[REDACTED]$3")
	return value
}

var (
	secretAssignmentRE = regexp.MustCompile(`(?i)(encryption_secret|password|token|client_secret)\s*[:=]\s*[^\s,;]+`)
	urlCredentialRE    = regexp.MustCompile(`(?i)(https?://)([^/@\s]+)(@)`)
)
