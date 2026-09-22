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
