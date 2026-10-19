package taskwarrior

import (
	"fmt"
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
