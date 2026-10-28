package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hoursFixture is Wednesday 14 October 2026: a 15 minute stand-up, an hour
// marked free at 13:00, and an all-day event.
const hoursFixture = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:standup
DTSTART:20261014T093000
DTEND:20261014T094500
SUMMARY:Team stand-up
END:VEVENT
BEGIN:VEVENT
UID:reminder
DTSTART:20261014T130000
DTEND:20261014T140000
TRANSP:TRANSPARENT
SUMMARY:Focus reminder
END:VEVENT
BEGIN:VEVENT
UID:offsite
DTSTART;VALUE=DATE:20261014
DTEND;VALUE=DATE:20261015
SUMMARY:Offsite
END:VEVENT
END:VCALENDAR
`

// calendarOnFile is a config with the calendar on over hoursFixture, last
// changed at modified, and the keys of [calendar] that follow.
func calendarOnFile(t *testing.T, modified time.Time, extra string) *configFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "work.ics")
	if err := os.WriteFile(path, []byte(hoursFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return newConfigFile(t, backendTable+"[calendar]\nenabled = true\npaths = [\""+path+"\"]\n"+extra)
}

func TestSettingsShowTheCalendarDefaultsForKeysTheFileLeavesOut(t *testing.T) {
	got := settingsOf(t, calendarOnFile(t, testNow, "").server()).Calendar
	if got.WorkingStart != "09:00" || got.WorkingEnd != "17:00" || got.IncludeAllDay || got.IncludeTransparent || got.StaleAfter != "24h" {
		t.Fatalf("calendar=%+v", got)
	}
}

func TestSavedWorkingHoursAndEventChoicesApplyAtOnce(t *testing.T) {
	file := calendarOnFile(t, testNow, "")
	server := file.server()
	if got := loadOf(t, server).Plan.CalendarMinutes; got != 15 {
		t.Fatalf("busy=%d, want the stand-up alone", got)
	}
	revision := settingsOf(t, server).Revision

	// An event shown as free counts once the switch says so.
	free := true
	got := saved(t, server, "saveCalendar", saveCalendarParams{Revision: revision, IncludeTransparent: &free})
	if !got.Calendar.IncludeTransparent || got.Revision == revision {
		t.Fatalf("the answer should show the new value and revision: %+v", got)
	}
	if busy := loadOf(t, server).Plan.CalendarMinutes; busy != 75 {
		t.Fatalf("busy=%d, want the stand-up and the free hour", busy)
	}

	// Working hours cut what counts: only 13:30 to 14:00 of the free hour is left.
	start := "13:30"
	got = saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, WorkingStart: &start})
	if got.Calendar.WorkingStart != "13:30" || got.Calendar.WorkingEnd != "17:00" {
		t.Fatalf("calendar=%+v", got.Calendar)
	}
	if busy := loadOf(t, server).Plan.CalendarMinutes; busy != 30 {
		t.Fatalf("busy=%d, want the last half hour of the free hour", busy)
	}

	// An all-day event takes the whole working day, whatever the hours are.
	allDay := true
	got = saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, IncludeAllDay: &allDay})
	if busy := loadOf(t, server).Plan.CalendarMinutes; busy != 210 {
		t.Fatalf("busy=%d, want 13:30 to 17:00", busy)
	}

	end := "15:30"
	saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, WorkingEnd: &end})
	if busy := loadOf(t, server).Plan.CalendarMinutes; busy != 120 {
		t.Fatalf("busy=%d, want 13:30 to 15:30", busy)
	}
	text := contents(t, file.path)
	for _, want := range []string{`working_start = "13:30"`, `working_end = "15:30"`, "include_all_day = true", "include_transparent = true"} {
		if !strings.Contains(text, want) {
			t.Errorf("the file should hold %s:\n%s", want, text)
		}
	}
}

func TestAnInvalidCalendarValueIsRefusedOnItsFieldAndNothingIsWritten(t *testing.T) {
	file := calendarOnFile(t, testNow, `working_start = "09:00"`+"\n")
	server := file.server()
	revision := settingsOf(t, server).Revision
	before := contents(t, file.path)
	on := true

	for name, tc := range map[string]struct {
		params  saveCalendarParams
		field   string
		message string
	}{
		"an end before the start":  {saveCalendarParams{WorkingEnd: ptr("08:00")}, "workingHours", "Working hours must end after they start."},
		"a start at the end":       {saveCalendarParams{WorkingStart: ptr("17:00")}, "workingHours", "Working hours must end after they start."},
		"a time that is not one":   {saveCalendarParams{WorkingStart: ptr("9am")}, "workingHours", "Enter a time of day, such as 09:00."},
		"a length that is not one": {saveCalendarParams{StaleAfter: ptr("soon")}, "staleAfter", "Enter a length of time, such as 8h or 7h30m."},
		"a negative length":        {saveCalendarParams{StaleAfter: ptr("-1h")}, "staleAfter", "Stale after can’t be negative."},
	} {
		tc.params.Revision = revision
		tc.params.IncludeAllDay = &on // valid, and must not be written either
		refusal := refused(t, server, "saveCalendar", tc.params)
		if refusal.Field != tc.field || refusal.Message != tc.message {
			t.Errorf("%s: refusal=%+v", name, refusal)
		}
		if contents(t, file.path) != before {
			t.Fatalf("%s: nothing may be written, not even the valid switch", name)
		}
	}
}

func TestMovingBothWorkingHoursInOneSaveIsCheckedAsAPair(t *testing.T) {
	server := calendarOnFile(t, testNow, "").server()
	revision := settingsOf(t, server).Revision
	got := saved(t, server, "saveCalendar", saveCalendarParams{Revision: revision, WorkingStart: ptr("18:00"), WorkingEnd: ptr("19:00")})
	if got.Calendar.WorkingStart != "18:00" || got.Calendar.WorkingEnd != "19:00" {
		t.Fatalf("calendar=%+v", got.Calendar)
	}
}

func TestTheStaleLimitChangesWhatEachFileSays(t *testing.T) {
	server := calendarOnFile(t, testNow.Add(-3*time.Hour), "").server()
	got := settingsOf(t, server)
	if got.Calendar.Files[0].Status != "available" {
		t.Fatalf("a file 3 hours old is fine under the default 24h: %+v", got.Calendar.Files)
	}

	got = saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, StaleAfter: ptr("2h")})
	if got.Calendar.StaleAfter != "2h" || got.Calendar.Files[0].Status != "stale" {
		t.Fatalf("calendar=%+v", got.Calendar)
	}

	got = saved(t, server, "saveCalendar", saveCalendarParams{Revision: got.Revision, StaleAfter: ptr("0m")})
	if got.Calendar.StaleAfter != "0m" || got.Calendar.Files[0].Status != "available" {
		t.Fatalf("0m means a file is never stale: %+v", got.Calendar)
	}
}

func ptr[T any](value T) *T { return &value }
