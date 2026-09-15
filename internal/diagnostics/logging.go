package diagnostics

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jvrviegas/momentum/internal/taskwarrior"
)

// LogOptions makes debug-log paths testable and keeps process environment out
// of the logging implementation.
type LogOptions struct {
	HomeDir      string
	XDGStateHome string
	Output       io.Writer
}

// NewLogger creates a structured logger. Non-debug mode discards records and
// never creates a file.
func NewLogger(debug bool, options LogOptions) (*slog.Logger, io.Closer, string, error) {
	if !debug {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), nopCloser{}, "", nil
	}
	if options.Output != nil {
		return slog.New(slog.NewTextHandler(options.Output, &slog.HandlerOptions{Level: slog.LevelDebug})), nopCloser{}, "", nil
	}
	path := LogPath(options.HomeDir, options.XDGStateHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, path, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, path, err
	}
	return slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug})), file, path, nil
}

// LogCommand records safe operation metadata without argv/task descriptions.
func LogCommand(logger *slog.Logger, kind string, result taskwarrior.CommandResult, err error) {
	if logger == nil {
		return
	}
	attrs := []any{"kind", kind, "duration", result.Duration, "exit_code", result.ExitCode}
	if err != nil {
		attrs = append(attrs, "error", taskwarrior.Redact(err.Error()))
		logger.Error("taskwarrior command", attrs...)
		return
	}
	logger.Debug("taskwarrior command", attrs...)
}

func LogTransition(logger *slog.Logger, from, to, reason string) {
	if logger != nil {
		logger.Debug("state transition", "from", from, "to", to, "reason", reason, "at", time.Now().UTC())
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
