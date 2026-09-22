package diagnostics

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/config"
)

func TestDoctorReportsCalendarSourceWithoutEventContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calendar.ics")
	if err := os.WriteFile(path, []byte("BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Private meeting\nEND:VEVENT\nEND:VCALENDAR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = []string{path}
	report := Run(context.Background(), DoctorOptions{Config: settings, HomeDir: t.TempDir(), StateDir: t.TempDir(), Terminal: "xterm", LookPath: func(string) (string, error) { return "task", nil }})
	var output bytes.Buffer
	report.Print(&output)
	text := output.String()
	if !strings.Contains(text, "Calendar source") || !strings.Contains(text, "1 local ICS source") || strings.Contains(text, "Private meeting") {
		t.Fatalf("doctor output=%q", text)
	}
}
