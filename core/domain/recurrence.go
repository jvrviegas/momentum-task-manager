package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RecurrenceDefinition is the small, validated recurrence contract Momentum
// sends to Taskwarrior. Anchor is the first due date; Taskwarrior owns all
// subsequent generation and completion semantics.
type RecurrenceDefinition struct {
	Expression string
	Anchor     time.Time
}

var recurrenceIntervalPattern = regexp.MustCompile(`^(\d+)(days?|wks?|weeks?|mos?|months?)$`)

// ParseRecurrence validates the common Taskwarrior recurrence vocabulary used
// by Momentum. Unknown expressions are rejected rather than being silently
// passed through as a second recurrence language.
func ParseRecurrence(value string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.TrimPrefix(text, "recur:")
	text = strings.TrimSpace(text)
	if text == "" || text == "none" {
		return "", nil
	}
	switch text {
	case "daily", "day":
		return "daily", nil
	case "weekdays", "weekday":
		return "weekdays", nil
	case "weekly", "week":
		return "weekly", nil
	case "biweekly", "fortnight":
		return "2wks", nil
	case "monthly", "month":
		return "monthly", nil
	case "quarterly":
		return "3mo", nil
	case "semiannual":
		return "6mo", nil
	case "annual", "yearly", "year":
		return "1yr", nil
	}
	if match := recurrenceIntervalPattern.FindStringSubmatch(strings.ReplaceAll(text, " ", "")); match != nil {
		count, err := strconv.Atoi(match[1])
		if err != nil || count < 1 {
			return "", fmt.Errorf("recurrence interval must be positive")
		}
		unit := match[2]
		switch {
		case strings.HasPrefix(unit, "day"):
			return strconv.Itoa(count) + "days", nil
		case strings.HasPrefix(unit, "wk"), strings.HasPrefix(unit, "week"):
			return strconv.Itoa(count) + "wks", nil
		case strings.HasPrefix(unit, "mo"), strings.HasPrefix(unit, "month"):
			return strconv.Itoa(count) + "mo", nil
		}
	}
	return "", fmt.Errorf("unsupported recurrence %q; use daily, weekdays, weekly, a weekday, monthly, or an interval such as 2 weeks", value)
}

// ParseRecurrencePhrase maps a supported natural-language recurrence phrase to
// one Taskwarrior expression and a deterministic first due date.
func ParseRecurrencePhrase(value string, now time.Time) (RecurrenceDefinition, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.TrimPrefix(text, "every ")
	text = strings.TrimSpace(text)
	if text == "" {
		return RecurrenceDefinition{}, fmt.Errorf("recurrence phrase is empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	loc := now.Location()
	anchor := midnight(now.In(loc))
	if weekday, ok := parseWeekday(text); ok {
		anchor = nextWeekday(anchor, weekday)
		return RecurrenceDefinition{Expression: "weekly", Anchor: anchor}, nil
	}
	expression, err := ParseRecurrence(text)
	if err != nil {
		return RecurrenceDefinition{}, err
	}
	return RecurrenceDefinition{Expression: expression, Anchor: anchor}, nil
}

func parseWeekday(value string) (time.Weekday, bool) {
	weekdays := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday,
		"saturday": time.Saturday,
	}
	weekday, ok := weekdays[strings.ToLower(strings.TrimSpace(value))]
	return weekday, ok
}

func nextWeekday(from time.Time, wanted time.Weekday) time.Time {
	from = midnight(from)
	delta := (int(wanted) - int(from.Weekday()) + 7) % 7
	return from.AddDate(0, 0, delta)
}

// RecurrenceNext returns the next anchor after value for the supported
// expressions. Its calendar and duration rules mirror Taskwarrior 3.x so the
// preview does not claim a different occurrence from the native recurrence
// engine.
func RecurrenceNext(value time.Time, expression string) (time.Time, bool) {
	canonical, err := ParseRecurrence(expression)
	if err != nil || value.IsZero() {
		return time.Time{}, false
	}
	switch canonical {
	case "daily":
		return value.Add(24 * time.Hour), true
	case "weekdays":
		candidate := value.Add(24 * time.Hour)
		for candidate.Weekday() == time.Saturday || candidate.Weekday() == time.Sunday {
			candidate = candidate.Add(24 * time.Hour)
		}
		return candidate, true
	case "weekly":
		return value.Add(7 * 24 * time.Hour), true
	case "monthly":
		return addCalendarMonthsClamped(value, 1), true
	case "1yr":
		return addCalendarMonthsClamped(value, 12), true
	}
	match := recurrenceIntervalPattern.FindStringSubmatch(canonical)
	if match == nil {
		return time.Time{}, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil || count < 1 {
		return time.Time{}, false
	}
	var unit time.Duration
	switch {
	case strings.HasPrefix(match[2], "day"):
		unit = 24 * time.Hour
	case strings.HasPrefix(match[2], "wk"), strings.HasPrefix(match[2], "week"):
		unit = 7 * 24 * time.Hour
	case strings.HasPrefix(match[2], "mo"), strings.HasPrefix(match[2], "month"):
		unit = 30 * 24 * time.Hour
	default:
		return time.Time{}, false
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if int64(count) > int64(maxDuration/unit) {
		return time.Time{}, false
	}
	return value.Add(time.Duration(count) * unit), true
}

// NextInstanceDue returns the due date of the instance that follows one due
// at after, in a backend series whose template is due at anchor and repeats
// by rule (backend plan D5). It is the first occurrence later than after that
// falls on or after the start of now's day, so occurrences missed in between
// are skipped. now also supplies the time zone: occurrences keep the anchor's
// wall-clock time there across DST changes, and month rules count calendar
// months from the anchor, ending on the last day of a shorter month. A time
// that a DST change skips moves forward by the change; of a time it repeats,
// the first is used. Unlike RecurrenceNext, which follows Taskwarrior, no
// rule moves by a fixed duration. ok is false for an unsupported rule, and
// when the next occurrence would be after the year 9999.
func NextInstanceDue(anchor time.Time, rule string, after, now time.Time) (time.Time, bool) {
	step, ok := seriesStepOf(rule)
	if !ok {
		return time.Time{}, false
	}
	loc := now.Location()
	today, err := startOfDay(civilDate(now), loc)
	if err != nil {
		return time.Time{}, false
	}
	first := civilTime(anchor.In(loc))
	from := civilDate(now)
	if day := civilDate(after.In(loc)); day.After(from) {
		from = day
	}
	for n := step.index(first, from); ; n++ {
		date, ok := step.occurrence(first, n)
		if date.Year() > 9999 {
			return time.Time{}, false
		}
		if !ok {
			continue
		}
		if occurrence := wallClockOrLater(date, loc); occurrence.After(after) && !occurrence.Before(today) {
			return occurrence, true
		}
	}
}

// MoveWithDue returns value moved along with a due date that moves from due
// to next, at the same wall-clock distance from it in next's time zone. The
// backend moves an instance's scheduled and wait dates this way, as
// Taskwarrior moves them, so a date-only wait stays on its day across a DST
// change.
func MoveWithDue(value, due, next time.Time) time.Time {
	loc := next.Location()
	offset := civilTime(value.In(loc)).Sub(civilTime(due.In(loc)))
	return wallClockOrLater(civilTime(next).Add(offset), loc)
}

// maxIntervalCount bounds a series' interval count. A longer interval cannot
// land before the year 10000, and its arithmetic could overflow.
const maxIntervalCount = 10000 * 366

// seriesStep is how far apart a series' occurrences are, in days or calendar
// months. A weekdays series has an occurrence on each Monday to Friday after
// its anchor.
type seriesStep struct {
	days, months int
	weekdays     bool
}

func seriesStepOf(rule string) (seriesStep, bool) {
	canonical, err := ParseRecurrence(rule)
	if err != nil {
		return seriesStep{}, false
	}
	switch canonical {
	case "daily":
		return seriesStep{days: 1}, true
	case "weekdays":
		return seriesStep{days: 1, weekdays: true}, true
	case "weekly":
		return seriesStep{days: 7}, true
	case "monthly":
		return seriesStep{months: 1}, true
	case "1yr":
		return seriesStep{months: 12}, true
	}
	match := recurrenceIntervalPattern.FindStringSubmatch(canonical)
	if match == nil {
		return seriesStep{}, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil || count > maxIntervalCount {
		return seriesStep{}, false
	}
	switch {
	case strings.HasPrefix(match[2], "day"):
		return seriesStep{days: count}, true
	case strings.HasPrefix(match[2], "wk"):
		return seriesStep{days: 7 * count}, true
	default:
		return seriesStep{months: count}, true
	}
}

// index returns the number of an occurrence no later than any on or after
// the civil date day, counting the anchor, first, as 0.
func (s seriesStep) index(first, day time.Time) int {
	if s.months > 0 {
		months := (day.Year()-first.Year())*12 + int(day.Month()) - int(first.Month())
		return max(0, months/s.months)
	}
	// Unix seconds, because a time.Duration cannot span 300 years.
	days := int(day.Unix()-civilDate(first).Unix()) / (24 * 60 * 60)
	return max(0, days/s.days)
}

// occurrence returns the civil time of occurrence n, and whether the series
// has one then: a weekdays series skips weekends after its anchor.
func (s seriesStep) occurrence(first time.Time, n int) (time.Time, bool) {
	if s.months > 0 {
		return addCalendarMonthsClamped(first, n*s.months), true
	}
	date := first.AddDate(0, 0, n*s.days)
	weekend := date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
	return date, !s.weekdays || n == 0 || !weekend
}

// civilTime returns the wall-clock time t shows, in UTC fields.
func civilTime(t time.Time) time.Time {
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	return time.Date(year, month, day, hour, minute, second, t.Nanosecond(), time.UTC)
}

// civilDate returns the date t shows, in UTC fields.
func civilDate(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// wallClockOrLater returns the instant at which loc's clocks read civil, the
// first when a DST change repeats it. When a change skips civil, it returns
// the instant the clocks would have read civil without the change, as far
// after the change as civil is after its start: 01:30 lost to a jump from
// 01:00 to 02:00 becomes 02:30.
func wallClockOrLater(civil time.Time, loc *time.Location) time.Time {
	if instants := wallClockInstants(civil, loc); len(instants) > 0 {
		return instants[0]
	}
	_, offset := civil.Add(-36 * time.Hour).In(loc).Zone()
	return civil.Add(-time.Duration(offset) * time.Second).In(loc)
}

func addCalendarMonthsClamped(value time.Time, months int) time.Time {
	year, month, day := value.Date()
	target := time.Date(year, month, 1, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location()).AddDate(0, months, 0)
	lastDay := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, value.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(target.Year(), target.Month(), day, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}

func midnight(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

// RecurrenceTargetUUID identifies the template when Taskwarrior exported a
// generated instance. It is intentionally only a routing hint; callers must
// still explain template/instance policy before mutating recurrence fields.
func (t Task) RecurrenceTargetUUID() string {
	if t.Parent != "" {
		return t.Parent
	}
	return t.UUID
}

func (t Task) IsRecurrenceTemplate() bool {
	return strings.EqualFold(t.Status, "recurring")
}

func (t Task) IsRecurrenceInstance() bool { return t.Parent != "" }

// RecurrenceDisplay returns a human-readable policy label without hiding an
// expression Taskwarrior supplied outside Momentum's preset set.
func RecurrenceDisplay(expression string) string {
	canonical, err := ParseRecurrence(expression)
	if err != nil {
		return strings.TrimSpace(expression)
	}
	switch canonical {
	case "daily":
		return "Daily"
	case "weekdays":
		return "Weekdays"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	case "2wks":
		return "Every 2 weeks"
	case "3mo":
		return "Quarterly"
	case "6mo":
		return "Every 6 months"
	case "1yr":
		return "Annual"
	default:
		return "Every " + canonical
	}
}
