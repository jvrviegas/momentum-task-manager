package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ptr[T any](value T) *T { return &value }

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const notesConfig = `# my notes
refresh_interval = "2m"

[backend]
url = "https://momentum.example"

[planning]
enabled = true
daily_capacity = "8h" # a full day
`

func TestSavePlanningChangesOnlyItsKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.SavePlanning(context.Background(), snapshot, PlanningPatch{
		Enabled: ptr(false), DailyCapacity: ptr("7h30m"), WeekdayCapacity: map[string]string{"Friday": "4h"},
	})
	if err != nil {
		t.Fatal(err)
	}

	text := readFile(t, path)
	for _, kept := range []string{"# my notes", `refresh_interval = "2m"`, `url = "https://momentum.example"`, "# a full day"} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q should be kept:\n%s", kept, text)
		}
	}
	settings, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Planning.Enabled || settings.Planning.DailyCapacity != 7*time.Hour+30*time.Minute ||
		settings.Planning.CapacityFor(time.Friday) != 4*time.Hour || settings.Planning.CapacityFor(time.Monday) != 7*time.Hour+30*time.Minute {
		t.Fatalf("planning=%+v", settings.Planning)
	}
	if settings.RefreshInterval != 2*time.Minute || settings.Backend.URL != "https://momentum.example" {
		t.Fatalf("unrelated keys changed: %+v", settings)
	}
}

func TestSavePlanningCreatesWhatIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "momentum", "config.toml")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePlanning(context.Background(), snapshot, PlanningPatch{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadFile(path)
	if err != nil || settings.Planning.Enabled {
		t.Fatalf("settings=%+v err=%v", settings.Planning, err)
	}
}

func TestAnEmptyWeekdayGoesBackToTheDailyCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig+"\n[planning.weekday_capacity]\nfriday = \"4h\"\n")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePlanning(context.Background(), snapshot, PlanningPatch{WeekdayCapacity: map[string]string{"friday": "  "}}); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadFile(path)
	if err != nil || settings.Planning.CapacityFor(time.Friday) != 8*time.Hour {
		t.Fatalf("friday=%v err=%v", settings.Planning.CapacityFor(time.Friday), err)
	}
}

func TestAFileThatChangedSinceItWasReadIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := notesConfig + "# edited in the terminal app\n"
	writeFile(t, path, elsewhere)

	_, err = store.SavePlanning(context.Background(), snapshot, PlanningPatch{Enabled: ptr(false)})
	if !errors.Is(err, ErrProjectConfigConflict) {
		t.Fatalf("err=%v, want a conflict", err)
	}
	if got := readFile(t, path); got != elsewhere {
		t.Fatalf("the file was overwritten:\n%s", got)
	}
}

func TestSavePlanningKeepsASymlinkASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles-config.toml")
	link := filepath.Join(dir, "config.toml")
	writeFile(t, target, notesConfig)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	store := NewProjectCatalogStore(link)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePlanning(context.Background(), snapshot, PlanningPatch{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config.toml is no longer a symlink: %v %v", info, err)
	}
	if !strings.Contains(readFile(t, target), "enabled = false") {
		t.Fatalf("the target was not changed:\n%s", readFile(t, target))
	}
}

func TestPlanningPatchRefusesWhatTheConfigRefuses(t *testing.T) {
	for name, test := range map[string]struct {
		patch PlanningPatch
		field string
		text  string
	}{
		"words, not a length":    {PlanningPatch{DailyCapacity: ptr("a day")}, "dailyCapacity", "Enter a length of time"},
		"a number with no unit":  {PlanningPatch{DailyCapacity: ptr("8")}, "dailyCapacity", "Enter a length of time"},
		"zero capacity":          {PlanningPatch{DailyCapacity: ptr("0m")}, "dailyCapacity", "longer than zero"},
		"negative buffer":        {PlanningPatch{Buffer: ptr("-1h")}, "buffer", "can’t be negative"},
		"a weekday of zero":      {PlanningPatch{WeekdayCapacity: map[string]string{"monday": "0h"}}, "weekdayCapacity.monday", "Leave it empty"},
		"a weekday not a length": {PlanningPatch{WeekdayCapacity: map[string]string{"friday": "half"}}, "weekdayCapacity.friday", "Enter a length of time"},
		"not a weekday":          {PlanningPatch{WeekdayCapacity: map[string]string{"someday": "4h"}}, "weekdayCapacity.someday", "weekday name"},
	} {
		got := test.patch.Check()
		if got == nil || got.Field != test.field || !strings.Contains(got.Message, test.text) {
			t.Errorf("%s: got %+v, want field %q containing %q", name, got, test.field, test.text)
		}
	}
	for name, ok := range map[string]PlanningPatch{
		"hours and minutes": {DailyCapacity: ptr("7h30m")},
		"a zero buffer":     {Buffer: ptr("0m")},
		"an empty weekday":  {WeekdayCapacity: map[string]string{"monday": ""}},
		"only the switch":   {Enabled: ptr(false)},
	} {
		if got := ok.Check(); got != nil {
			t.Errorf("%s: got %+v, want nothing", name, got)
		}
	}
}

func TestAnInvalidPlanningPatchWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig)
	store := NewProjectCatalogStore(path)
	snapshot, _ := store.Read(context.Background())
	_, err := store.SavePlanning(context.Background(), snapshot, PlanningPatch{Enabled: ptr(false), Buffer: ptr("-1h")})
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "buffer" {
		t.Fatalf("err=%v", err)
	}
	if readFile(t, path) != notesConfig {
		t.Fatal("a refused patch must not change the file, not even for its valid keys")
	}
}

func TestLengthsAreTypedTheShortWay(t *testing.T) {
	for want, length := range map[string]time.Duration{
		"8h": 8 * time.Hour, "7h30m": 7*time.Hour + 30*time.Minute, "45m": 45 * time.Minute, "0m": 0, "1h5m": time.Hour + 5*time.Minute,
	} {
		if got := FormatLength(length); got != want {
			t.Errorf("FormatLength(%v)=%q, want %q", length, got, want)
		}
		if parsed, err := ParseLength(want); err != nil || parsed != length {
			t.Errorf("ParseLength(%q)=%v,%v", want, parsed, err)
		}
	}
}

func TestCalendarFilesAreKeptWithATildeAndOnce(t *testing.T) {
	home := "/Users/someone"
	patch := CalendarPatch{Paths: ptr([]string{
		"/Users/someone/Calendars/work.ics", "  ", "~/Calendars/work.ics", "/Volumes/Shared/team.ics", "/Users/someoneelse/a.ics",
	})}.Normalized(home)
	want := []string{"~/Calendars/work.ics", "/Volumes/Shared/team.ics", "/Users/someoneelse/a.ics"}
	if got := *patch.Paths; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("paths=%v, want %v", got, want)
	}
}

func TestCalendarPatchRefusesWhatTheConfigRefuses(t *testing.T) {
	off := CalendarConfig{}
	on := CalendarConfig{Enabled: true, Paths: []string{"~/a.ics"}}
	for name, test := range map[string]struct {
		patch   CalendarPatch
		current CalendarConfig
		field   string
	}{
		"switching on with no files":      {CalendarPatch{Enabled: ptr(true)}, off, "enabled"},
		"removing the last file while on": {CalendarPatch{Paths: ptr([]string{})}, on, "paths"},
		"a web address":                   {CalendarPatch{Paths: ptr([]string{"https://example.test/a.ics"})}, off, "paths"},
	} {
		got := test.patch.Check(test.current)
		if got == nil || got.Field != test.field {
			t.Errorf("%s: got %+v, want field %q", name, got, test.field)
		}
	}
	for name, test := range map[string]struct {
		patch   CalendarPatch
		current CalendarConfig
	}{
		"adding a file while off":      {CalendarPatch{Paths: ptr([]string{"~/a.ics"})}, off},
		"switching on with a file":     {CalendarPatch{Enabled: ptr(true)}, on},
		"switching on and adding":      {CalendarPatch{Enabled: ptr(true), Paths: ptr([]string{"~/a.ics"})}, off},
		"removing a file, one is left": {CalendarPatch{Paths: ptr([]string{"~/a.ics"})}, CalendarConfig{Enabled: true, Paths: []string{"~/a.ics", "~/b.ics"}}},
		"removing the last while off":  {CalendarPatch{Paths: ptr([]string{})}, CalendarConfig{Paths: []string{"~/a.ics"}}},
	} {
		if got := test.patch.Check(test.current); got != nil {
			t.Errorf("%s: got %+v, want nothing", name, got)
		}
	}
}

func TestSaveCalendarWritesTheSwitchAndTheFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCalendar(context.Background(), snapshot, CalendarPatch{
		Enabled: ptr(true), Paths: ptr([]string{"~/Calendars/work.ics", "/Volumes/Shared/team.ics"}),
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Calendar.Enabled || strings.Join(settings.Calendar.Paths, "|") != "~/Calendars/work.ics|/Volumes/Shared/team.ics" {
		t.Fatalf("calendar=%+v", settings.Calendar)
	}
	if !strings.Contains(readFile(t, path), "# my notes") {
		t.Fatal("comments should be kept")
	}
}

func TestCalendarPatchChecksHoursAgainstWhatIsThere(t *testing.T) {
	current := CalendarConfig{WorkingStart: "09:00", WorkingEnd: "17:00"}
	for name, test := range map[string]CalendarPatch{
		"an end before the stored start": {WorkingEnd: ptr("08:30")},
		"a start after the stored end":   {WorkingStart: ptr("17:30")},
		"a start equal to the end":       {WorkingStart: ptr("17:00")},
		"a pair that ends first":         {WorkingStart: ptr("12:00"), WorkingEnd: ptr("11:00")},
	} {
		if got := test.Check(current); got == nil || got.Field != "workingHours" {
			t.Errorf("%s: got %+v, want workingHours", name, got)
		}
	}
	for name, test := range map[string]CalendarPatch{
		"an end after the stored start": {WorkingEnd: ptr("18:00")},
		"a pair moved together":         {WorkingStart: ptr("18:00"), WorkingEnd: ptr("19:30")},
		"a length of zero":              {StaleAfter: ptr("0m")},
		"a day":                         {StaleAfter: ptr("24h")},
	} {
		if got := test.Check(current); got != nil {
			t.Errorf("%s: got %+v, want nothing", name, got)
		}
	}
}

func TestSaveCalendarWritesHoursEventsAndTheStaleLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, notesConfig)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCalendar(context.Background(), snapshot, CalendarPatch{
		WorkingStart: ptr("08:30"), WorkingEnd: ptr("16:00"), IncludeAllDay: ptr(true), IncludeTransparent: ptr(false), StaleAfter: ptr("90m"),
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	calendar := settings.Calendar
	if calendar.WorkingStart != "08:30" || calendar.WorkingEnd != "16:00" || !calendar.IncludeAllDay || calendar.IncludeTransparent || calendar.StaleAfter != 90*time.Minute {
		t.Fatalf("calendar=%+v", calendar)
	}
	text := readFile(t, path)
	for _, kept := range []string{"# my notes", "# a full day", `refresh_interval = "2m"`} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q should be kept:\n%s", kept, text)
		}
	}
}
