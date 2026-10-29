package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCalendarSourceDefaultsToFiles(t *testing.T) {
	if source := Defaults().Calendar.Source; source != CalendarSourceFiles {
		t.Fatalf("default source = %q, want %q", source, CalendarSourceFiles)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, "[calendar]\nenabled = false\n")
	settings, err := LoadFile(path)
	if err != nil || settings.Calendar.Source != CalendarSourceFiles {
		t.Fatalf("a file with no source: %q, %v; want the default", settings.Calendar.Source, err)
	}
}

func TestCalendarSourceLoadsFromTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, "[calendar]\nsource = \"google\"\nworking_start = \"08:00\"\n")
	settings, err := LoadFile(path)
	if err != nil || settings.Calendar.Source != CalendarSourceGoogle || settings.Calendar.WorkingStart != "08:00" {
		t.Fatalf("LoadFile = %+v, %v", settings.Calendar, err)
	}
}

func TestGoogleNeedsNoPathsAndCountsWhateverEnabledSays(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		settings := Defaults()
		settings.Calendar.Source = CalendarSourceGoogle
		settings.Calendar.Enabled = enabled
		settings.Calendar.Paths = nil
		if err := settings.Validate(); err != nil {
			t.Errorf("enabled %v with Google and no files: %v", enabled, err)
		}
	}
}

func TestGoogleStillChecksTheWorkingHoursAndTheStaleLimit(t *testing.T) {
	for name, change := range map[string]func(*CalendarConfig){
		"hours that end before they start": func(c *CalendarConfig) { c.WorkingStart, c.WorkingEnd = "17:00", "09:00" },
		"a start that is not a time":       func(c *CalendarConfig) { c.WorkingStart = "morning" },
		"a negative stale limit":           func(c *CalendarConfig) { c.StaleAfter = -time.Hour },
	} {
		// Enabled is off: for files that would skip every check, but a
		// Google source is on whatever it says.
		settings := Defaults()
		settings.Calendar.Source = CalendarSourceGoogle
		settings.Calendar.Enabled = false
		change(&settings.Calendar)
		if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "calendar.") {
			t.Errorf("%s: %v, want an error naming the key", name, err)
		}
	}
}

func TestAnUnknownCalendarSourceIsRefused(t *testing.T) {
	settings := Defaults()
	settings.Calendar.Source = "icloud"
	err := settings.Validate()
	if err == nil || !strings.Contains(err.Error(), `"calendar.source"`) || !strings.Contains(err.Error(), `"icloud"`) {
		t.Fatalf("Validate = %v, want an error naming calendar.source and the value", err)
	}
}

func TestFilesKeepTodaysRules(t *testing.T) {
	for _, source := range []string{CalendarSourceFiles, ""} {
		settings := Defaults()
		settings.Calendar.Source = source
		settings.Calendar.Enabled = true
		if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "calendar.paths") {
			t.Errorf("source %q, enabled, no paths: %v, want the paths error", source, err)
		}
		settings.Calendar.Paths = []string{"https://example.test/calendar.ics"}
		if err := settings.Validate(); err == nil {
			t.Errorf("source %q: a remote path was accepted", source)
		}
		// Off, nothing is checked, as before.
		settings.Calendar.Enabled = false
		settings.Calendar.WorkingStart, settings.Calendar.WorkingEnd = "17:00", "09:00"
		if err := settings.Validate(); err != nil {
			t.Errorf("source %q, disabled: %v", source, err)
		}
	}
}

func TestCalendarPatchChecksTheSource(t *testing.T) {
	googleOn := CalendarConfig{Source: CalendarSourceGoogle, Enabled: true, WorkingStart: "09:00", WorkingEnd: "17:00"}
	filesOn := CalendarConfig{Source: CalendarSourceFiles, Enabled: true, Paths: []string{"~/a.ics"}, WorkingStart: "09:00", WorkingEnd: "17:00"}

	// Google needs no files, so nothing about files is refused.
	if err := (CalendarPatch{Source: ptr(CalendarSourceGoogle)}).Check(filesOn); err != nil {
		t.Errorf("choosing Google: %v", err)
	}
	if err := (CalendarPatch{WorkingStart: ptr("08:00")}).Check(googleOn); err != nil {
		t.Errorf("changing the hours of a Google source with no files: %v", err)
	}
	if err := (CalendarPatch{Paths: ptr([]string{})}).Check(googleOn); err != nil {
		t.Errorf("emptying the files of a Google source: %v", err)
	}
	// Choosing files again, with the switch on and nothing to read, is refused.
	if err := (CalendarPatch{Source: ptr(CalendarSourceFiles)}).Check(googleOn); err == nil || err.Field != "source" ||
		!strings.Contains(err.Message, "Add a calendar file first") {
		t.Errorf("choosing files with none: %v", err)
	}
	// With files to read it is fine, and so is choosing files with the switch off.
	if err := (CalendarPatch{Source: ptr(CalendarSourceFiles), Paths: ptr([]string{"~/a.ics"})}).Check(googleOn); err != nil {
		t.Errorf("choosing files and adding one in the same save: %v", err)
	}
	if err := (CalendarPatch{Source: ptr(CalendarSourceFiles), Enabled: ptr(false)}).Check(googleOn); err != nil {
		t.Errorf("choosing files with the switch off: %v", err)
	}
	// Nothing but the two sources is accepted.
	if err := (CalendarPatch{Source: ptr("icloud")}).Check(filesOn); err == nil || err.Field != "source" {
		t.Errorf("an unknown source: %v", err)
	}
	// The old rules for files stand.
	if err := (CalendarPatch{Paths: ptr([]string{})}).Check(filesOn); err == nil || err.Field != "paths" {
		t.Errorf("removing the last file while on: %v", err)
	}
}

func TestSaveCalendarWritesTheSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, "# mine\n[calendar]\nenabled = true # on\npaths = [\"~/a.ics\"]\n")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCalendar(context.Background(), snapshot, CalendarPatch{Source: ptr(CalendarSourceGoogle)}); err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	for _, kept := range []string{"# mine", "enabled = true # on", `paths = ["~/a.ics"]`, `source = "google"`} {
		if !strings.Contains(text, kept) {
			t.Errorf("the file lost %q:\n%s", kept, text)
		}
	}
	settings, err := LoadFile(path)
	if err != nil || settings.Calendar.Source != CalendarSourceGoogle {
		t.Fatalf("LoadFile = %q, %v; want google", settings.Calendar.Source, err)
	}
	// A save that leaves the source out leaves it as it is.
	snapshot, _ = store.Read(context.Background())
	if _, err := store.SaveCalendar(context.Background(), snapshot, CalendarPatch{WorkingStart: ptr("08:00")}); err != nil {
		t.Fatal(err)
	}
	if text := readFile(t, path); !strings.Contains(text, `source = "google"`) {
		t.Errorf("a save without the source dropped it:\n%s", text)
	}
}
