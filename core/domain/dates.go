package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ResolveDate turns a due or scheduled expression into an absolute instant.
// now supplies both the clock and the time zone. The vocabulary is what
// Momentum suggests and produces plus the named dates Taskwarrior 3 documents,
// except its holidays; anything else is rejected rather than guessed (backend
// plan D3, T03). Results agree with Taskwarrior 3.5 except where it is wrong:
// next-week stays on Friday midnight across a DST change, an ordinal day
// missing from its month is an error, and a typed time that a DST change skips
// or repeats is an error.
func ResolveDate(expression string, now time.Time) (time.Time, error) {
	text := strings.TrimSpace(expression)
	if text == "" {
		return time.Time{}, errors.New("date is empty")
	}
	loc := now.Location()
	if value, ok, err := resolveTimestamp(text, loc); ok {
		return value, err
	}
	name := strings.ToLower(text)
	year, month, day := now.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	switch name {
	case "now":
		return now.Truncate(time.Second), nil
	case "today", "sod":
		return startOfDay(today, loc)
	case "eod":
		return endOfDay(today, loc)
	case "yesterday":
		return startOfDay(today.AddDate(0, 0, -1), loc)
	case "tomorrow":
		return startOfDay(today.AddDate(0, 0, 1), loc)
	case "next-week":
		// Friday of the next calendar week (DESIGN, Quick add).
		return startOfDay(startOfWeek(today).AddDate(0, 0, 11), loc)
	case "later", "someday":
		return startOfDay(time.Date(9999, 12, 30, 0, 0, 0, 0, time.UTC), loc)
	}
	key, end := name, false
	if rest, ok := strings.CutPrefix(name, "eo"); ok {
		key, end = "so"+rest, true
	}
	if period, ok := periodNames[key]; ok {
		first, last := period.days(today)
		if end {
			return endOfDay(last, loc)
		}
		return startOfDay(first, loc)
	}
	if weekday, ok := matchName(weekdayNames, name); ok {
		// Always after today, as in Taskwarrior.
		ahead := (weekday - int(today.Weekday()) + 7) % 7
		if ahead == 0 {
			ahead = 7
		}
		return startOfDay(today.AddDate(0, 0, ahead), loc)
	}
	if index, ok := matchName(monthNames, name); ok {
		next := time.Date(year, time.Month(index+1), 1, 0, 0, 0, 0, time.UTC)
		if next.Month() <= month {
			next = next.AddDate(1, 0, 0)
		}
		return startOfDay(next, loc)
	}
	if number, ok := parseOrdinal(name); ok {
		target := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		if number <= day {
			target = target.AddDate(0, 1, 0)
		}
		if number > daysIn(target) {
			return time.Time{}, fmt.Errorf("the %s does not exist in %s", name, target.Format("January 2006"))
		}
		return startOfDay(target.AddDate(0, 0, number-1), loc)
	}
	return time.Time{}, fmt.Errorf("unsupported or invalid date %q; use today, tomorrow, a weekday, next-week, YYYY-MM-DD, or YYYY-MM-DD HH:MM", expression)
}

// resolveTimestamp reads written-out dates. ok is false when text is not one,
// so the caller tries named dates next.
func resolveTimestamp(text string, loc *time.Location) (time.Time, bool, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00", "20060102T150405Z07:00"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.In(loc), true, nil
		}
	}
	if parsed, err := time.Parse(time.DateOnly, text); err == nil {
		value, err := startOfDay(parsed, loc)
		return value, true, err
	}
	// The editor's form, ISO 8601 without a zone, and the capture
	// interpreter's form are wall-clock times in loc.
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02T15:04:05", "20060102T150405"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			value, err := wallClock(parsed, loc)
			return value, true, err
		}
	}
	return time.Time{}, false, nil
}

// wallClock returns the one instant at which loc's clocks read civil, whose
// UTC fields hold the civil time.
func wallClock(civil time.Time, loc *time.Location) (time.Time, error) {
	instants := wallClockInstants(civil, loc)
	switch len(instants) {
	case 0:
		return time.Time{}, fmt.Errorf("%s does not exist in %s; correct the time", civil.Format("2006-01-02 15:04"), loc)
	case 1:
		return instants[0], nil
	default:
		return time.Time{}, fmt.Errorf("%s is ambiguous in %s; correct the time", civil.Format("2006-01-02 15:04"), loc)
	}
}

// wallClockInstants returns every instant, earliest first, at which loc's
// clocks read civil: none when a DST change skips it, two when one repeats it.
// It tries each offset in force within a day and a half of civil, which
// covers any one transition.
func wallClockInstants(civil time.Time, loc *time.Location) []time.Time {
	var instants []time.Time
	for probe := -36 * time.Hour; probe <= 36*time.Hour; probe += 6 * time.Hour {
		_, offset := civil.Add(probe).In(loc).Zone()
		instant := civil.Add(-time.Duration(offset) * time.Second).In(loc)
		year, month, day := instant.Date()
		hour, minute, second := instant.Clock()
		if !time.Date(year, month, day, hour, minute, second, 0, time.UTC).Equal(civil) || slices.ContainsFunc(instants, instant.Equal) {
			continue
		}
		instants = append(instants, instant)
	}
	slices.SortFunc(instants, func(a, b time.Time) int { return a.Compare(b) })
	return instants
}

// startOfDay returns the first instant of the civil date day in loc. When a
// DST change skips midnight, the day starts at the change, as in Taskwarrior.
func startOfDay(day time.Time, loc *time.Location) (time.Time, error) {
	for minute := range 24 * 60 {
		if instants := wallClockInstants(day.Add(time.Duration(minute)*time.Minute), loc); len(instants) > 0 {
			return instants[0], nil
		}
	}
	return time.Time{}, fmt.Errorf("%s does not exist in %s", day.Format(time.DateOnly), loc)
}

// endOfDay returns the last whole second of the civil date day in loc.
func endOfDay(day time.Time, loc *time.Location) (time.Time, error) {
	last := day.Add(24*time.Hour - time.Second)
	for minute := range 24 * 60 {
		if instants := wallClockInstants(last.Add(-time.Duration(minute)*time.Minute), loc); len(instants) > 0 {
			return instants[len(instants)-1], nil
		}
	}
	return time.Time{}, fmt.Errorf("%s does not exist in %s", day.Format(time.DateOnly), loc)
}

type periodUnit int

const (
	periodWeek periodUnit = iota
	periodWorkWeek
	periodMonth
	periodQuarter
	periodYear
)

// period is a calendar period, offset whole periods from the current one.
type period struct {
	unit   periodUnit
	offset int
}

// periodNames lists Taskwarrior's documented start-of-period names; each
// end-of-period name swaps "so" for "eo". soww, eoww, sopd, eopd, sond, and
// eond work in Taskwarrior 3.5 but are undocumented, so they are rejected.
var periodNames = map[string]period{
	"sow": {periodWeek, 0}, "sopw": {periodWeek, -1}, "sonw": {periodWeek, 1},
	"sopww": {periodWorkWeek, -1}, "sonww": {periodWorkWeek, 1},
	"som": {periodMonth, 0}, "sopm": {periodMonth, -1}, "sonm": {periodMonth, 1},
	"soq": {periodQuarter, 0}, "sopq": {periodQuarter, -1}, "sonq": {periodQuarter, 1},
	"soy": {periodYear, 0}, "sopy": {periodYear, -1}, "sony": {periodYear, 1},
}

// days returns the first and last civil dates of the period around today.
// Weeks start on Monday.
func (p period) days(today time.Time) (time.Time, time.Time) {
	switch p.unit {
	case periodWeek, periodWorkWeek:
		first := startOfWeek(today).AddDate(0, 0, 7*p.offset)
		if p.unit == periodWorkWeek {
			return first, first.AddDate(0, 0, 4)
		}
		return first, first.AddDate(0, 0, 6)
	case periodMonth:
		first := time.Date(today.Year(), today.Month()+time.Month(p.offset), 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(0, 1, -1)
	case periodQuarter:
		firstMonth := (today.Month()-1)/3*3 + 1
		first := time.Date(today.Year(), firstMonth+time.Month(3*p.offset), 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(0, 3, -1)
	default: // periodYear
		first := time.Date(today.Year()+p.offset, time.January, 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(1, 0, -1)
	}
}

func startOfWeek(day time.Time) time.Time {
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

func daysIn(firstOfMonth time.Time) int {
	return firstOfMonth.AddDate(0, 1, -1).Day()
}

var (
	weekdayNames = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	monthNames   = []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
)

// matchName finds the name that text abbreviates. As Taskwarrior documents
// for weekdays and months, any prefix of three or more letters counts.
func matchName(names []string, text string) (int, bool) {
	if len(text) < 3 {
		return 0, false
	}
	for index, name := range names {
		if strings.HasPrefix(name, text) {
			return index, true
		}
	}
	return 0, false
}

var ordinalPattern = regexp.MustCompile(`^([1-9]\d?)(st|nd|rd|th)$`)

// parseOrdinal reads a day of the month such as 1st, 22nd, or 13th, with the
// suffix English uses for that number.
func parseOrdinal(text string) (int, bool) {
	match := ordinalPattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	number, _ := strconv.Atoi(match[1])
	if number > 31 {
		return 0, false
	}
	suffix := "th"
	if number%100 < 11 || number%100 > 13 {
		switch number % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return number, match[2] == suffix
}
