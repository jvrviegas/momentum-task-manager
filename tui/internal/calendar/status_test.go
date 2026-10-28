package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const oneMeeting = "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:1\nSUMMARY:Secret meeting title\nDTSTART:20261014T100000Z\nDTEND:20261014T110000Z\nEND:VEVENT\nEND:VCALENDAR\n"

func TestInspectSaysWhatEachFileIs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	write := func(name, text string, age time.Duration) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
		return path
	}
	folder := filepath.Join(dir, "a-folder.ics")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		path   string
		state  FileState
		detail string
	}{
		"a fresh file":             {write("fresh.ics", oneMeeting, time.Hour), FileAvailable, ""},
		"a file nobody refreshed":  {write("old.ics", oneMeeting, 3*24*time.Hour), FileStale, ""},
		"a file that is not there": {filepath.Join(dir, "gone.ics"), FileMissing, ""},
		"a folder":                 {folder, FileUnreadable, "is a directory"},
		"an event that never ends": {write("broken.ics", "BEGIN:VEVENT\nSUMMARY:x\n", time.Hour), FileUnreadable, "unterminated VEVENT"},
		"an event with no start":   {write("nostart.ics", "BEGIN:VEVENT\nSUMMARY:x\nEND:VEVENT\n", time.Hour), FileUnreadable, "DTSTART"},
	} {
		got := Inspect(test.path, now, 24*time.Hour)
		if got.State != test.state || !strings.Contains(got.Detail, test.detail) {
			t.Errorf("%s: got %+v, want %s containing %q", name, got, test.state, test.detail)
		}
		if strings.Contains(got.Detail, dir) || strings.Contains(got.Detail, "Secret") {
			t.Errorf("%s: the detail %q should hold no path and no title", name, got.Detail)
		}
	}
	if got := Inspect(filepath.Join(dir, "fresh.ics"), now, 24*time.Hour); got.Modified.IsZero() {
		t.Error("an existing file says when it last changed")
	}
	if got := Inspect(filepath.Join(dir, "old.ics"), now, 0); got.State != FileAvailable {
		t.Errorf("a stale_after of zero turns the check off, got %+v", got)
	}
}
