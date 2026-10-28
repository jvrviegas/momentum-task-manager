package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// call sends one operation straight to the server's dispatcher.
func call(t *testing.T, server *Server, op string, params any) (any, error) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return server.dispatch(context.Background(), op, raw)
}

func settingsOf(t *testing.T, server *Server) *settingsResult {
	t.Helper()
	result, err := server.dispatch(context.Background(), "settings", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(*settingsResult)
}

func saved(t *testing.T, server *Server, op string, params any) *settingsResult {
	t.Helper()
	result, err := call(t, server, op, params)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return result.(*settingsResult)
}

func refused(t *testing.T, server *Server, op string, params any) *Error {
	t.Helper()
	result, err := call(t, server, op, params)
	var refusal *Error
	if !errors.As(err, &refusal) {
		t.Fatalf("%s: got %v, %v; want a refusal", op, result, err)
	}
	return refusal
}

func contents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSettingsShowTheValuesInUseAndEachCalendarFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, "Calendars")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	meeting := "BEGIN:VEVENT\nDTSTART:20261014T100000Z\nDTEND:20261014T110000Z\nEND:VEVENT\n"
	for name, age := range map[string]time.Duration{"fresh.ics": time.Hour, "old.ics": 72 * time.Hour} {
		path := filepath.Join(folder, name)
		if err := os.WriteFile(path, []byte(meeting), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, testNow.Add(-age), testNow.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(folder, "folder.ics"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := newConfigFile(t, backendTable+`[planning]
daily_capacity = "7h30m"
buffer = "45m"
[planning.weekday_capacity]
friday = "4h"
[calendar]
enabled = true
paths = ["~/Calendars/fresh.ics", "~/Calendars/old.ics", "~/Calendars/gone.ics", "~/Calendars/folder.ics"]
`)
	got := settingsOf(t, file.server())

	if !got.Planning.Enabled || got.Planning.DailyCapacity != "7h30m" || got.Planning.Buffer != "45m" || got.Planning.WeekdayCapacity["friday"] != "4h" {
		t.Fatalf("planning=%+v", got.Planning)
	}
	if got.Revision == "" || got.ConfigPath != file.path {
		t.Fatalf("revision=%q path=%q", got.Revision, got.ConfigPath)
	}
	if !got.Calendar.Enabled || len(got.Calendar.Files) != 4 {
		t.Fatalf("calendar=%+v", got.Calendar)
	}
	for i, want := range []struct{ path, status, detail string }{
		{"~/Calendars/fresh.ics", "available", ""},
		{"~/Calendars/old.ics", "stale", ""},
		{"~/Calendars/gone.ics", "missing", ""},
		{"~/Calendars/folder.ics", "error", "is a directory"},
	} {
		file := got.Calendar.Files[i]
		if file.Path != want.path || file.Status != want.status || !strings.Contains(file.Detail, want.detail) {
			t.Errorf("file %d = %+v, want %+v", i, file, want)
		}
	}
	if got.Calendar.Files[0].Modified == "" || got.Calendar.Files[2].Modified != "" {
		t.Errorf("only an existing file says when it changed: %+v", got.Calendar.Files)
	}
}

func TestSettingsAnswerBeforeAServerIsNamedAndAsLists(t *testing.T) {
	file := newConfigFile(t, "[planning]\ndaily_capacity = \"6h\"\n")
	server := file.server()
	responses := serve(t, server, request(t, 1, "settings", nil))
	var result map[string]any
	if err := json.Unmarshal(responses[1].Result, &result); err != nil || responses[1].Error != nil {
		t.Fatalf("response=%+v err=%v", responses[1], err)
	}
	if files, ok := result["calendar"].(map[string]any)["files"].([]any); !ok || len(files) != 0 {
		t.Errorf("files should be an empty list, got %v", result["calendar"])
	}
	if days, ok := result["planning"].(map[string]any)["weekdayCapacity"].(map[string]any); !ok || len(days) != 0 {
		t.Errorf("weekdayCapacity should be an empty object, got %v", result["planning"])
	}
}

func TestASavedPlanningChangeAppliesAtOnce(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 360 {
		t.Fatalf("capacity=%d", got)
	}
	revision := settingsOf(t, server).Revision

	enabled, capacity := false, "7h"
	got := saved(t, server, "savePlanning", savePlanningParams{Revision: revision, Enabled: &enabled, DailyCapacity: &capacity})
	if got.Planning.Enabled || got.Planning.DailyCapacity != "7h" || got.Revision == revision {
		t.Fatalf("the answer should show the new values and a new revision: %+v", got)
	}
	if loadOf(t, server).Plan.Enabled {
		t.Fatal("the next load should already see planning off")
	}
	if text := contents(t, file.path); !strings.Contains(text, "enabled = false") || !strings.Contains(text, `daily_capacity = "7h"`) {
		t.Fatalf("the file should hold the change:\n%s", text)
	}

	on := true
	saved(t, server, "savePlanning", savePlanningParams{Revision: got.Revision, Enabled: &on})
	if got := loadOf(t, server).Plan.CapacityMinutes; got != 420 {
		t.Fatalf("capacity=%d, want the saved 420 with planning back on", got)
	}
}

func TestASaveOnAFileThatChangedElsewhereIsRefused(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	revision := settingsOf(t, server).Revision
	file.write(planning7h + "# added in an editor\n")
	elsewhere := contents(t, file.path)

	off := false
	refusal := refused(t, server, "savePlanning", savePlanningParams{Revision: revision, Enabled: &off})
	if refusal.Code != CodeConfigConflict || !strings.Contains(refusal.Message, "changed outside Momentum") {
		t.Fatalf("refusal=%+v", refusal)
	}
	if contents(t, file.path) != elsewhere {
		t.Fatal("a refused save must leave the file as it is")
	}
	if got := settingsOf(t, server); got.Planning.DailyCapacity != "7h" || got.Revision == revision {
		t.Fatalf("settings should show what the file says now: %+v", got)
	}
}

func TestAnInvalidLengthIsRefusedOnItsFieldAndNothingIsWritten(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	revision := settingsOf(t, server).Revision
	before := contents(t, file.path)

	off, bad := false, "a day"
	refusal := refused(t, server, "savePlanning", savePlanningParams{Revision: revision, Enabled: &off, DailyCapacity: &bad})
	if refusal.Field != "dailyCapacity" || !strings.Contains(refusal.Message, "8h or 7h30m") {
		t.Fatalf("refusal=%+v", refusal)
	}
	if contents(t, file.path) != before {
		t.Fatal("not even the valid switch may be written")
	}
}

func TestSaveIsRefusedWhileTheFileDoesNotLoad(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	revision := settingsOf(t, server).Revision
	file.write(backendTable + "[calendar]\nenabled = true\n")
	if status := statusOf(t, server); status.ConfigError == "" {
		t.Fatal("the file should not load")
	}
	before := contents(t, file.path)

	off := false
	refusal := refused(t, server, "savePlanning", savePlanningParams{Revision: revision, Enabled: &off})
	if !strings.Contains(refusal.Message, "Fix config.toml first") {
		t.Fatalf("refusal=%+v", refusal)
	}
	if contents(t, file.path) != before {
		t.Fatal("the broken file must not be touched")
	}
}

func TestCalendarFilesAreKeptWithATildeAndTheSwitchNeedsOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	file := newConfigFile(t, planning6h)
	server := file.server()
	revision := settingsOf(t, server).Revision
	on, off := true, false

	// Switching on with no files is refused, with the config as it was.
	before := contents(t, file.path)
	if refusal := refused(t, server, "saveCalendar", saveCalendarParams{Revision: revision, Enabled: &on}); refusal.Field != "enabled" || refusal.Message != "Add a calendar file first." {
		t.Fatalf("refusal=%+v", refusal)
	}
	if contents(t, file.path) != before {
		t.Fatal("a refused switch must not be written")
	}

	// A file under home is stored with ~/, a repeat only once.
	paths := []string{filepath.Join(home, "Calendars", "work.ics"), "~/Calendars/work.ics", "/Volumes/Shared/team.ics"}
	got := saved(t, server, "saveCalendar", saveCalendarParams{Revision: revision, Paths: &paths, Enabled: &on})
	if !got.Calendar.Enabled || len(got.Calendar.Files) != 2 || got.Calendar.Files[0].Path != "~/Calendars/work.ics" || got.Calendar.Files[1].Path != "/Volumes/Shared/team.ics" {
		t.Fatalf("calendar=%+v", got.Calendar)
	}
	if got.Calendar.Files[0].Status != "missing" || got.Calendar.Files[1].Status != "missing" {
		t.Fatalf("files that are not there say so: %+v", got.Calendar.Files)
	}
	if !strings.Contains(contents(t, file.path), "~/Calendars/work.ics") {
		t.Fatal("the file should hold the ~/ path")
	}

	// Removing the last file while it is on is refused; off first, then it goes.
	none := []string{}
	if refusal := refused(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, Paths: &none}); refusal.Field != "paths" {
		t.Fatalf("refusal=%+v", refusal)
	}
	next := saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, Enabled: &off, Paths: &none})
	if next.Calendar.Enabled || len(next.Calendar.Files) != 0 {
		t.Fatalf("calendar=%+v", next.Calendar)
	}
}
