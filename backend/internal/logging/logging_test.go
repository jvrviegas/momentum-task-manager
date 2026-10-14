package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestNewWritesCloudLoggingFields(t *testing.T) {
	var out bytes.Buffer
	logger := New(&out)
	logger.Warn("request", slog.String("method", "GET"), slog.String("route", "GET /health"), slog.Int("status", 200))

	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("log line %q: %v", out.String(), err)
	}
	for key, want := range map[string]any{"severity": "WARNING", "message": "request", "method": "GET", "route": "GET /health", "status": 200.0} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v in %s", key, line[key], want, out.String())
		}
	}
	if _, ok := line["time"]; !ok {
		t.Errorf("no time in %s", out.String())
	}
	if _, ok := line["level"]; ok {
		t.Errorf("level not renamed in %s", out.String())
	}
}

func TestNewRedactsAttributesOutsideTheAllowlist(t *testing.T) {
	var out bytes.Buffer
	logger := New(&out)
	logger.Error("request failed",
		slog.String("description", "secret plans"),
		slog.String("authorization", "Bearer secret-token"),
		slog.Any("error", errors.New(`modify "secret plans"`)),
		slog.Group("task", slog.String("project", "secret.project"), slog.String("status", "pending")),
		slog.Int("status", 500),
	)
	logger.With(slog.String("email", "secret@example.com")).Info("signed in")

	for _, secret := range []string{"secret plans", "secret-token", "secret.project", "secret@example.com"} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, out.String())
		}
	}
	// An allowlisted key inside a group is still redacted: only top-level
	// attributes are checked against the allowlist.
	if !strings.Contains(out.String(), `"task":{"project":"[redacted]","status":"[redacted]"}`) ||
		!strings.Contains(out.String(), `"status":500`) || strings.Count(out.String(), "\n") != 2 {
		t.Errorf("unexpected log:\n%s", out.String())
	}
}
