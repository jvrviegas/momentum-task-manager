package calendar

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixture = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:a@example
DTSTART:20260921T100000Z
DTEND:20260921T110000Z
SUMMARY:First
END:VEVENT
BEGIN:VEVENT
UID:b@example
DTSTART:20260921T103000Z
DTEND:20260921T120000Z
SUMMARY:Overlap
END:VEVENT
BEGIN:VEVENT
UID:all@example
DTSTART;VALUE=DATE:20260921
DTEND;VALUE=DATE:20260922
SUMMARY:Holiday
END:VEVENT
BEGIN:VEVENT
UID:free@example
DTSTART:20260921T130000Z
DTEND:20260921T140000Z
TRANSP:TRANSPARENT
SUMMARY:Free
END:VEVENT
END:VCALENDAR
`

func TestLoadAndMergeOverlappingICSBusyIntervals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calendar.ics")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	result := Load(context.Background(), Options{Paths: []string{path}, IncludeAllDay: true, Now: now})
	if result.Err != nil || len(result.Events) != 3 {
		t.Fatalf("result=%#v", result)
	}
	intervals, err := BusyIntervals(result.Events, now, "09:00", "17:00", false)
	if err != nil || len(intervals) != 1 || BusyMinutes(intervals) != 120 {
		t.Fatalf("intervals=%#v err=%v minutes=%d", intervals, err, BusyMinutes(intervals))
	}
	intervals, err = BusyIntervals(result.Events, now, "09:00", "17:00", true)
	if err != nil || len(intervals) != 1 || BusyMinutes(intervals) != 480 {
		t.Fatalf("all-day intervals=%#v err=%v minutes=%d", intervals, err, BusyMinutes(intervals))
	}
	transparentResult := Load(context.Background(), Options{Paths: []string{path}, IncludeAllDay: true, IncludeTransparent: true, Now: now})
	intervals, err = BusyIntervalsWithOptions(transparentResult.Events, now, "09:00", "17:00", false, true)
	if err != nil || BusyMinutes(intervals) != 180 {
		t.Fatalf("transparent intervals=%#v err=%v minutes=%d", intervals, err, BusyMinutes(intervals))
	}
}

func TestRecurringICSExpansionAndDeduplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calendar.ics")
	data := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:weekly@example
DTSTART;TZID=UTC:20260918T100000
DTEND;TZID=UTC:20260918T110000
RRULE:FREQ=WEEKLY;BYDAY=MO,FR
SUMMARY:Weekly
END:VEVENT
END:VCALENDAR
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Load(context.Background(), Options{Paths: []string{path}, Now: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)})
	if result.Err != nil || len(result.Events) == 0 {
		t.Fatalf("result=%#v", result)
	}
	if result.Events[0].Summary != "Weekly" {
		t.Fatalf("events=%#v", result.Events)
	}
}

func TestMissingICSIsUnavailableWithoutReturningEventContent(t *testing.T) {
	result := Load(context.Background(), Options{Paths: []string{"/missing/calendar.ics"}, Now: time.Now()})
	if result.Err == nil || len(result.Events) != 0 {
		t.Fatalf("result=%#v", result)
	}
}
