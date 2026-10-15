package domain

import (
	"testing"
	"time"
)

func TestParseRecurrencePresetsAndIntervals(t *testing.T) {
	for input, want := range map[string]string{
		"daily": "daily", "weekdays": "weekdays", "weekly": "weekly", "biweekly": "2wks",
		"monthly": "monthly", "2 weeks": "2wks", "3 months": "3mo",
	} {
		got, err := ParseRecurrence(input)
		if err != nil || got != want {
			t.Errorf("ParseRecurrence(%q)=%q err=%v want %q", input, got, err, want)
		}
	}
}

func TestParseRecurrencePhraseUsesAnchor(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)
	got, err := ParseRecurrencePhrase("every Friday", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Expression != "weekly" || got.Anchor.Weekday() != time.Friday || got.Anchor.Day() != 25 {
		t.Fatalf("definition=%#v", got)
	}
}

func TestRecurrenceNextMatchesTaskwarriorCalendarSemantics(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		anchor     time.Time
		expression string
		want       time.Time
	}{
		{
			name:       "daily preserves instant across DST",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "daily",
			want:       time.Date(2027, 3, 28, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "weekdays skip weekend and preserve instant",
			anchor:     time.Date(2027, 3, 26, 15, 0, 0, 0, lisbon),
			expression: "weekdays",
			want:       time.Date(2027, 3, 29, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "weekly preserves instant across DST",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "weekly",
			want:       time.Date(2027, 4, 3, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly preserves wall clock",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2027, 4, 27, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly clamps month end",
			anchor:     time.Date(2027, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2027, 2, 28, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly clamps leap year month end",
			anchor:     time.Date(2028, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2028, 2, 29, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "month interval is thirty day duration",
			anchor:     time.Date(2026, 12, 31, 15, 0, 0, 0, lisbon),
			expression: "2mo",
			want:       time.Date(2027, 3, 1, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "quarterly is ninety day duration",
			anchor:     time.Date(2027, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "quarterly",
			want:       time.Date(2027, 5, 1, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "annual clamps leap day",
			anchor:     time.Date(2028, 2, 29, 15, 0, 0, 0, lisbon),
			expression: "annual",
			want:       time.Date(2029, 2, 28, 15, 0, 0, 0, lisbon),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := RecurrenceNext(test.anchor, test.expression)
			if !ok || !got.Equal(test.want) || got.Location() != test.want.Location() {
				t.Fatalf("RecurrenceNext(%v, %q)=%v, %v; want %v", test.anchor, test.expression, got, ok, test.want)
			}
		})
	}
}

func TestRecurrenceTargetDistinguishesGeneratedInstance(t *testing.T) {
	task := Task{UUID: "instance", Parent: "template", Status: "pending", Recurrence: "daily"}
	if !task.IsRecurrenceInstance() || task.IsRecurrenceTemplate() || task.RecurrenceTargetUUID() != "template" {
		t.Fatalf("task=%#v", task)
	}
}

func TestNextInstanceDue(t *testing.T) {
	lisbon, santiago := location(t, "Europe/Lisbon"), location(t, "America/Santiago")
	in := func(loc *time.Location, year int, month time.Month, day, hour, minute int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, loc)
	}
	lisbonAt := func(year int, month time.Month, day, hour int) time.Time {
		return in(lisbon, year, month, day, hour, 0)
	}
	tests := []struct {
		name             string
		anchor           time.Time
		rule             string
		after, now, want time.Time
	}{
		// Lisbon's clocks go forward from 01:00 to 02:00 on 2027-03-28 and
		// back from 02:00 to 01:00 on 2027-10-31. Taskwarrior 3.5 moves daily
		// and weekly series by 24 hours or 7 days, so the wall clock moves too.
		{"daily keeps the wall clock across DST", lisbonAt(2027, 3, 26, 9), "daily",
			lisbonAt(2027, 3, 27, 9), lisbonAt(2027, 3, 27, 10), lisbonAt(2027, 3, 28, 9)},
		{"weekly keeps the wall clock across DST", lisbonAt(2027, 10, 25, 9), "weekly",
			lisbonAt(2027, 10, 25, 9), lisbonAt(2027, 10, 25, 9), lisbonAt(2027, 11, 1, 9)},
		{"the zone comes from now", lisbonAt(2027, 3, 26, 9), "daily",
			lisbonAt(2027, 3, 27, 9), lisbonAt(2027, 3, 27, 10).UTC(), lisbonAt(2027, 3, 28, 10)},
		{"day intervals count days", lisbonAt(2027, 3, 26, 9), "3 days",
			lisbonAt(2027, 3, 26, 9), lisbonAt(2027, 3, 26, 9), lisbonAt(2027, 3, 29, 9)},
		{"weekdays skip the weekend", lisbonAt(2027, 9, 3, 15), "weekdays",
			lisbonAt(2027, 9, 3, 15), lisbonAt(2027, 9, 3, 15), lisbonAt(2027, 9, 6, 15)},
		{"a weekday series anchored on a weekend goes on to Monday", lisbonAt(2027, 9, 4, 15), "weekdays",
			lisbonAt(2027, 9, 4, 15), lisbonAt(2027, 9, 4, 15), lisbonAt(2027, 9, 6, 15)},
		{"the anchor is an occurrence even on a weekend", lisbonAt(2027, 9, 4, 15), "weekdays",
			lisbonAt(2027, 9, 3, 15), lisbonAt(2027, 9, 3, 15), lisbonAt(2027, 9, 4, 15)},

		// Taskwarrior 3.5 keeps a clamped day (Mar 29 here) and counts
		// month intervals as 30 days (May 30 below).
		{"monthly clamps to a shorter month", lisbonAt(2028, 1, 31, 15), "monthly",
			lisbonAt(2028, 1, 31, 15), lisbonAt(2028, 1, 31, 15), lisbonAt(2028, 2, 29, 15)},
		{"monthly counts from the anchor, not the clamped day", lisbonAt(2028, 1, 31, 15), "monthly",
			lisbonAt(2028, 2, 29, 15), lisbonAt(2028, 2, 29, 16), lisbonAt(2028, 3, 31, 15)},
		{"month intervals count calendar months", lisbonAt(2028, 1, 31, 9), "2mo",
			lisbonAt(2028, 3, 31, 9), lisbonAt(2028, 3, 31, 9), lisbonAt(2028, 5, 31, 9)},
		{"quarterly counts calendar months", lisbonAt(2027, 11, 30, 9), "quarterly",
			lisbonAt(2027, 11, 30, 9), lisbonAt(2027, 11, 30, 9), lisbonAt(2028, 2, 29, 9)},
		{"annual clamps a leap day", lisbonAt(2028, 2, 29, 9), "annual",
			lisbonAt(2028, 2, 29, 9), lisbonAt(2028, 2, 29, 9), lisbonAt(2029, 2, 28, 9)},
		{"annual returns to a leap day", lisbonAt(2028, 2, 29, 9), "annual",
			lisbonAt(2031, 2, 28, 9), lisbonAt(2031, 2, 28, 9), lisbonAt(2032, 2, 29, 9)},

		// Missed occurrences are skipped (D5), but today's stays.
		{"a late close skips to today", lisbonAt(2027, 10, 1, 9), "daily",
			lisbonAt(2027, 10, 10, 9), lisbonAt(2027, 10, 14, 10), lisbonAt(2027, 10, 14, 9)},
		{"a late close keeps today's before its time", lisbonAt(2027, 10, 1, 9), "daily",
			lisbonAt(2027, 10, 10, 9), lisbonAt(2027, 10, 14, 8), lisbonAt(2027, 10, 14, 9)},
		{"a late weekly close skips a missed week and waits for its day", lisbonAt(2027, 10, 4, 9), "weekly",
			lisbonAt(2027, 10, 4, 9), lisbonAt(2027, 10, 13, 12), lisbonAt(2027, 10, 18, 9)},
		{"a late monthly close skips missed months and waits for its day", lisbonAt(2027, 1, 1, 9), "monthly",
			lisbonAt(2027, 6, 1, 9), lisbonAt(2027, 10, 14, 12), lisbonAt(2027, 11, 1, 9)},
		{"a late monthly close keeps today's", lisbonAt(2027, 1, 31, 9), "monthly",
			lisbonAt(2027, 6, 30, 9), lisbonAt(2027, 10, 31, 12), lisbonAt(2027, 10, 31, 9)},
		{"an early close moves past the closed instance", lisbonAt(2027, 10, 1, 9), "daily",
			lisbonAt(2027, 10, 16, 9), lisbonAt(2027, 10, 14, 12), lisbonAt(2027, 10, 17, 9)},
		{"an instance moved off the series moves the next one past it", lisbonAt(2027, 10, 1, 9), "daily",
			lisbonAt(2027, 10, 16, 14), lisbonAt(2027, 10, 14, 12), lisbonAt(2027, 10, 17, 9)},

		{"a time a DST change skips moves forward", in(lisbon, 2027, 3, 27, 1, 30), "daily",
			in(lisbon, 2027, 3, 27, 1, 30), in(lisbon, 2027, 3, 27, 1, 30), in(lisbon, 2027, 3, 28, 2, 30)},
		{"a time a DST change repeats is the first", in(lisbon, 2027, 10, 30, 1, 30), "daily",
			in(lisbon, 2027, 10, 30, 1, 30), in(lisbon, 2027, 10, 30, 1, 30), time.Date(2027, 10, 31, 0, 30, 0, 0, time.UTC)},
		// Santiago's clocks go forward from 00:00 to 01:00 on 2027-09-05.
		{"a skipped midnight becomes the change", in(santiago, 2027, 9, 4, 0, 0), "daily",
			in(santiago, 2027, 9, 4, 0, 0), in(santiago, 2027, 9, 4, 0, 0), in(santiago, 2027, 9, 5, 1, 0)},
		{"the day after a skipped midnight starts at midnight", in(santiago, 2027, 9, 4, 0, 0), "daily",
			in(santiago, 2027, 9, 5, 1, 0), in(santiago, 2027, 9, 5, 1, 0), in(santiago, 2027, 9, 6, 0, 0)},
		{"the last day before the year 10000", lisbonAt(9999, 12, 30, 0), "daily",
			lisbonAt(9999, 12, 30, 0), lisbonAt(9999, 12, 30, 0), lisbonAt(9999, 12, 31, 0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := NextInstanceDue(test.anchor, test.rule, test.after, test.now)
			if !ok || !got.Equal(test.want) || got.Location() != test.now.Location() {
				t.Errorf("NextInstanceDue = %v, %v; want %v", got, ok, test.want.In(test.now.Location()))
			}
		})
	}
}

func TestNextInstanceDueRefuses(t *testing.T) {
	anchor := time.Date(2027, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, rule string
		after      time.Time
	}{
		{"an unsupported rule", "hourly", anchor},
		{"no rule", "none", anchor},
		{"a day after 9999", "daily", time.Date(9999, 12, 31, 9, 0, 0, 0, time.UTC)},
		{"an interval past 9999", "500000 weeks", anchor},
		{"an interval too long to count", "9223372036854775807 weeks", anchor},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := NextInstanceDue(anchor, test.rule, test.after, anchor); ok {
				t.Errorf("NextInstanceDue = %v, want none", got)
			}
		})
	}
}

func TestMoveWithDueKeepsTheWallClockDistance(t *testing.T) {
	lisbon := location(t, "Europe/Lisbon")
	at := func(month time.Month, day, hour int) time.Time {
		return time.Date(2027, month, day, hour, 0, 0, 0, lisbon)
	}
	for _, test := range []struct {
		name                   string
		value, due, next, want time.Time
	}{
		// 33 hours before the due date is Saturday 23:00 in the week the
		// clocks go forward, as Taskwarrior 3.5 would have it.
		{"a date-only wait stays on its day across DST", at(3, 21, 0), at(3, 22, 9), at(3, 29, 9), at(3, 28, 0)},
		{"scheduled the day before", at(10, 24, 8), at(10, 25, 9), at(11, 1, 9), at(10, 31, 8)},
		{"after the due date", at(10, 25, 18), at(10, 25, 9), at(11, 1, 9), at(11, 1, 18)},
		{"the same due date", at(10, 24, 8), at(10, 25, 9), at(10, 25, 9), at(10, 24, 8)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := MoveWithDue(test.value, test.due, test.next); !got.Equal(test.want) {
				t.Errorf("MoveWithDue = %v, want %v", got, test.want)
			}
		})
	}
}

func location(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
