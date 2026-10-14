// Package logging writes the server's structured logs.
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger with the severity, message, and time fields
// Cloud Logging reads. Only attributes whose keys are in allowed keep their
// values; every other value becomes "[redacted]", so task contents and tokens
// cannot reach the log by accident (ADR 0002, P3). Messages must be constant
// text for the same reason.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: replace}))
}

// allowed lists the attribute keys whose values are safe to log. Add a key
// only for values that can never hold task contents, tokens, or email
// addresses.
var allowed = map[string]bool{
	"addr":        true,
	"duration_ms": true,
	"method":      true,
	"route":       true,
	"status":      true,
}

func replace(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) == 0 {
		switch attr.Key {
		case slog.TimeKey:
			return attr
		case slog.LevelKey:
			return slog.String("severity", severity(attr.Value))
		case slog.MessageKey:
			return slog.String("message", attr.Value.String())
		}
		if allowed[attr.Key] {
			return attr
		}
	}
	return slog.String(attr.Key, "[redacted]")
}

func severity(value slog.Value) string {
	if level, ok := value.Any().(slog.Level); ok && level == slog.LevelWarn {
		return "WARNING"
	}
	return value.String()
}
