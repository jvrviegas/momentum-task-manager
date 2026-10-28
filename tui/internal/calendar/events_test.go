package calendar

import (
	"testing"
	"time"
)

func TestFromEventsNarrowsToTheDayAndIsNeverStale(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, time.Local) }
	events := []Event{
		{UID: "today", Start: at(14, 11), End: at(14, 12)},
		{UID: "today", Start: at(14, 11), End: at(14, 12)}, // the same occurrence twice
		{UID: "yesterday", Start: at(13, 11), End: at(13, 12)},
		{UID: "free", Start: at(14, 13), End: at(14, 14), Transparent: true},
		{UID: "all-day", Start: at(14, 0), End: at(15, 0), AllDay: true},
	}

	got := FromEvents(events, Options{Now: now})
	if len(got.Events) != 1 || got.Events[0].UID != "today" || got.Stale || got.Err != nil {
		t.Fatalf("result = %+v", got)
	}

	got = FromEvents(events, Options{Now: now, IncludeAllDay: true, IncludeTransparent: true, StaleAfter: time.Nanosecond})
	if len(got.Events) != 3 || got.Stale {
		t.Fatalf("result = %+v", got)
	}
}
