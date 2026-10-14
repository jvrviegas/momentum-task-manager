package domain

import (
	"strings"
	"testing"
	"time"
)

func loadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// The expected values are what `task calc` printed in Taskwarrior 3.5.0 at
// this instant and zone, except next-week, which Momentum sends to
// Taskwarrior as sow+1w+4d.
func TestResolveDateMatchesTaskwarriorNamedDates(t *testing.T) {
	lisbon := loadLocation(t, "Europe/Lisbon")
	now := time.Date(2026, 10, 14, 17, 8, 27, 500, lisbon)
	for expression, want := range map[string]string{
		"now": "2026-10-14T17:08:27", "today": "2026-10-14T00:00:00", "sod": "2026-10-14T00:00:00",
		"eod": "2026-10-14T23:59:59", "yesterday": "2026-10-13T00:00:00", "tomorrow": "2026-10-15T00:00:00",
		"next-week": "2026-10-23T00:00:00",
		"monday":    "2026-10-19T00:00:00", "mon": "2026-10-19T00:00:00", "tue": "2026-10-20T00:00:00",
		"tues": "2026-10-20T00:00:00", "wed": "2026-10-21T00:00:00", "wednesday": "2026-10-21T00:00:00",
		"wedn": "2026-10-21T00:00:00", "thu": "2026-10-15T00:00:00", "thurs": "2026-10-15T00:00:00",
		"friday": "2026-10-16T00:00:00", "Friday": "2026-10-16T00:00:00", "sunday": "2026-10-18T00:00:00",
		"january": "2027-01-01T00:00:00", "jun": "2027-06-01T00:00:00", "jul": "2027-07-01T00:00:00",
		"sept": "2027-09-01T00:00:00", "oct": "2027-10-01T00:00:00", "october": "2027-10-01T00:00:00",
		"nov": "2026-11-01T00:00:00", "november": "2026-11-01T00:00:00",
		"later": "9999-12-30T00:00:00", "someday": "9999-12-30T00:00:00",
		"sow": "2026-10-12T00:00:00", "eow": "2026-10-18T23:59:59",
		"sopw": "2026-10-05T00:00:00", "eopw": "2026-10-11T23:59:59",
		"sonw": "2026-10-19T00:00:00", "eonw": "2026-10-25T23:59:59",
		"sopww": "2026-10-05T00:00:00", "eopww": "2026-10-09T23:59:59",
		"sonww": "2026-10-19T00:00:00", "eonww": "2026-10-23T23:59:59",
		"som": "2026-10-01T00:00:00", "eom": "2026-10-31T23:59:59",
		"sopm": "2026-09-01T00:00:00", "eopm": "2026-09-30T23:59:59",
		"sonm": "2026-11-01T00:00:00", "eonm": "2026-11-30T23:59:59",
		"soq": "2026-10-01T00:00:00", "eoq": "2026-12-31T23:59:59",
		"sopq": "2026-07-01T00:00:00", "eopq": "2026-09-30T23:59:59",
		"sonq": "2027-01-01T00:00:00", "eonq": "2027-03-31T23:59:59",
		"soy": "2026-01-01T00:00:00", "eoy": "2026-12-31T23:59:59",
		"sopy": "2025-01-01T00:00:00", "eopy": "2025-12-31T23:59:59",
		"sony": "2027-01-01T00:00:00", "eony": "2027-12-31T23:59:59",
		"1st": "2026-11-01T00:00:00", "13th": "2026-11-13T00:00:00", "14th": "2026-11-14T00:00:00",
		"15th": "2026-10-15T00:00:00", "31st": "2026-10-31T00:00:00",
		"2026-10-20": "2026-10-20T00:00:00", "2026-10-20 09:30": "2026-10-20T09:30:00",
		"2026-10-20T09:30": "2026-10-20T09:30:00", "2026-10-20T09:30:45": "2026-10-20T09:30:45",
		"20261020T093000": "2026-10-20T09:30:00", "20261020T093000Z": "2026-10-20T10:30:00",
		"2026-10-20T09:30Z": "2026-10-20T10:30:00", "2026-10-20T09:30:00+05:00": "2026-10-20T05:30:00",
		"  tomorrow  ": "2026-10-15T00:00:00",
	} {
		got, err := ResolveDate(expression, now)
		if err != nil {
			t.Errorf("ResolveDate(%q) error: %v", expression, err)
			continue
		}
		if got.Location() != lisbon || got.Format("2006-01-02T15:04:05") != want {
			t.Errorf("ResolveDate(%q)=%v want %s in Europe/Lisbon", expression, got, want)
		}
	}
}

func TestResolveDateRejectsUnsupportedExpressions(t *testing.T) {
	now := time.Date(2026, 10, 14, 17, 8, 27, 0, time.UTC)
	for _, expression := range []string{
		"", "  ",
		// Holidays are documented by Taskwarrior but not supported.
		"easter", "goodfriday", "eastermonday", "ascension", "pentecost", "midsommar", "midsommarafton", "juhannus",
		// Work in Taskwarrior 3.5 but are undocumented.
		"soww", "eoww", "sopd", "eopd", "sond", "eond", "9am", "noon",
		// Only weekday and month names may be shortened, to three letters or more.
		"to", "tom", "yes", "yest", "lat", "some", "mo", "sa", "ju",
		// Arithmetic, durations, and other Taskwarrior forms.
		"3d", "now+2d", "sow+1w+4d", "eom-1d", "2026-W43", "2026-293", "1792000000",
		"1nd", "2st", "0th", "01st", "32nd", "11st", "12nd", "13rd",
		"2026-02-30", "2026-10-20T25:00", "next monday", "in 3 days", "next week",
	} {
		if got, err := ResolveDate(expression, now); err == nil {
			t.Errorf("ResolveDate(%q)=%v, want an error", expression, got)
		}
	}
}

func TestResolveDateUnsupportedMessageNamesSupportedForms(t *testing.T) {
	_, err := ResolveDate("easter", time.Now())
	if err == nil || err.Error() != `unsupported or invalid date "easter"; use today, tomorrow, a weekday, next-week, YYYY-MM-DD, or YYYY-MM-DD HH:MM` {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveDateAcrossDSTChanges(t *testing.T) {
	lisbon := loadLocation(t, "Europe/Lisbon")
	santiago := loadLocation(t, "America/Santiago")
	tests := []struct {
		name       string
		expression string
		now        time.Time
		want       string // RFC 3339 with the expected offset
	}{
		// Lisbon leaves summer time at 02:00 on 2026-10-25.
		{"next-week stays on Friday midnight", "next-week", time.Date(2026, 10, 21, 12, 0, 0, 0, lisbon), "2026-10-30T00:00:00Z"},
		{"tomorrow before the change", "tomorrow", time.Date(2026, 10, 21, 12, 0, 0, 0, lisbon), "2026-10-22T00:00:00+01:00"},
		{"weekday after the change", "monday", time.Date(2026, 10, 21, 12, 0, 0, 0, lisbon), "2026-10-26T00:00:00Z"},
		{"end of week on the change day", "eow", time.Date(2026, 10, 21, 12, 0, 0, 0, lisbon), "2026-10-25T23:59:59Z"},
		{"start of next week", "sonw", time.Date(2026, 10, 21, 12, 0, 0, 0, lisbon), "2026-10-26T00:00:00Z"},
		{"today on the change day", "today", time.Date(2026, 10, 25, 12, 0, 0, 0, lisbon), "2026-10-25T00:00:00+01:00"},
		{"eod on the change day", "eod", time.Date(2026, 10, 25, 12, 0, 0, 0, lisbon), "2026-10-25T23:59:59Z"},
		// Lisbon enters summer time at 01:00 on 2027-03-28.
		{"time after the gap", "2027-03-28 02:30", time.Date(2027, 3, 20, 12, 0, 0, 0, lisbon), "2027-03-28T02:30:00+01:00"},
		{"zoned time inside the gap", "2027-03-28T01:30:00Z", time.Date(2027, 3, 20, 12, 0, 0, 0, lisbon), "2027-03-28T02:30:00+01:00"},
		{"next-week across the spring change", "next-week", time.Date(2027, 3, 24, 12, 0, 0, 0, lisbon), "2027-04-02T00:00:00+01:00"},
		// Santiago skips 00:00–01:00 on 2026-09-06; the day starts at 01:00.
		{"date whose midnight is skipped", "2026-09-06", time.Date(2026, 9, 1, 12, 0, 0, 0, santiago), "2026-09-06T01:00:00-03:00"},
		{"tomorrow whose midnight is skipped", "tomorrow", time.Date(2026, 9, 5, 12, 0, 0, 0, santiago), "2026-09-06T01:00:00-03:00"},
		// Santiago repeats 23:00–24:00 on 2027-04-03; the day ends at the later 23:59:59.
		{"eod in a repeated hour", "eod", time.Date(2027, 4, 3, 12, 0, 0, 0, santiago), "2027-04-03T23:59:59-04:00"},
		{"day after the repeated hour", "tomorrow", time.Date(2027, 4, 3, 12, 0, 0, 0, santiago), "2027-04-04T00:00:00-04:00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveDate(test.expression, test.now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Format(time.RFC3339) != test.want {
				t.Fatalf("ResolveDate(%q)=%s want %s", test.expression, got.Format(time.RFC3339), test.want)
			}
		})
	}
}

func TestResolveDateRejectsTimesThatDSTSkipsOrRepeats(t *testing.T) {
	lisbon := loadLocation(t, "Europe/Lisbon")
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, lisbon)
	for expression, want := range map[string]string{
		"2027-03-28 01:30":  "2027-03-28 01:30 does not exist in Europe/Lisbon; correct the time",
		"2027-03-28T01:30":  "2027-03-28 01:30 does not exist in Europe/Lisbon; correct the time",
		"20270328T013000":   "2027-03-28 01:30 does not exist in Europe/Lisbon; correct the time",
		"2026-10-25 01:30":  "2026-10-25 01:30 is ambiguous in Europe/Lisbon; correct the time",
		"2026-10-25T01:00":  "2026-10-25 01:00 is ambiguous in Europe/Lisbon; correct the time",
		"20261025T015959":   "2026-10-25 01:59 is ambiguous in Europe/Lisbon; correct the time",
		"2026-10-25T01:30Z": "",
	} {
		_, err := ResolveDate(expression, now)
		if want == "" {
			if err != nil {
				t.Errorf("ResolveDate(%q) error: %v", expression, err)
			}
			continue
		}
		if err == nil || err.Error() != want {
			t.Errorf("ResolveDate(%q) err=%v want %q", expression, err, want)
		}
	}
}

func TestResolveDateCalendarEdges(t *testing.T) {
	tests := []struct {
		expression string
		now        time.Time
		want       string
	}{
		{"sunday", time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), "2026-10-25"},
		{"monday", time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), "2026-10-19"},
		{"next-week", time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), "2026-10-23"},
		{"next-week", time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), "2026-10-30"},
		{"next-week", time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC), "2027-01-08"},
		{"sow", time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), "2026-10-12"},
		{"tomorrow", time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), "2027-01-01"},
		{"december", time.Date(2026, 12, 5, 9, 0, 0, 0, time.UTC), "2027-12-01"},
		{"jan", time.Date(2026, 12, 5, 9, 0, 0, 0, time.UTC), "2027-01-01"},
		{"feb", time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC), "2026-02-01"},
		{"1st", time.Date(2026, 12, 15, 9, 0, 0, 0, time.UTC), "2027-01-01"},
		{"28th", time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC), "2026-02-28"},
		{"29th", time.Date(2028, 2, 10, 9, 0, 0, 0, time.UTC), "2028-02-29"},
		{"21st", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), "2026-11-21"},
		{"22nd", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), "2026-10-22"},
		{"23rd", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), "2026-10-23"},
		{"11th", time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), "2026-10-11"},
		{"sopq", time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), "2025-10-01"},
		{"eopq", time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), "2025-12-31"},
		{"sonm", time.Date(2026, 12, 15, 9, 0, 0, 0, time.UTC), "2027-01-01"},
		{"eopm", time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), "2026-02-28"},
		{"eonww", time.Date(2026, 12, 30, 9, 0, 0, 0, time.UTC), "2027-01-08"},
	}
	for _, test := range tests {
		got, err := ResolveDate(test.expression, test.now)
		if err != nil {
			t.Errorf("ResolveDate(%q, %v) error: %v", test.expression, test.now, err)
			continue
		}
		if got.Format(time.DateOnly) != test.want {
			t.Errorf("ResolveDate(%q, %v)=%v want %s", test.expression, test.now, got, test.want)
		}
	}
}

// Taskwarrior lets mktime roll a missing day into the next month; its
// documentation calls that an error, and so does Momentum.
func TestResolveDateRejectsOrdinalMissingFromMonth(t *testing.T) {
	for _, test := range []struct {
		expression string
		now        time.Time
		want       string
	}{
		{"31st", time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC), "the 31st does not exist in November 2026"},
		{"30th", time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC), "the 30th does not exist in February 2026"},
		{"29th", time.Date(2026, 1, 30, 9, 0, 0, 0, time.UTC), "the 29th does not exist in February 2026"},
	} {
		_, err := ResolveDate(test.expression, test.now)
		if err == nil || err.Error() != test.want {
			t.Errorf("ResolveDate(%q, %v) err=%v want %q", test.expression, test.now, err, test.want)
		}
	}
}

func TestResolveDateNowDropsSubseconds(t *testing.T) {
	now := time.Date(2026, 10, 14, 17, 8, 27, 999_999_999, time.UTC)
	got, err := ResolveDate("NOW", now)
	if err != nil || !got.Equal(time.Date(2026, 10, 14, 17, 8, 27, 0, time.UTC)) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveDateRejectsADaySkippedEntirely(t *testing.T) {
	apia := loadLocation(t, "Pacific/Apia")
	_, err := ResolveDate("2011-12-30", time.Date(2011, 12, 1, 12, 0, 0, 0, apia))
	if err == nil || !strings.Contains(err.Error(), "2011-12-30 does not exist in Pacific/Apia") {
		t.Fatalf("err=%v", err)
	}
}
