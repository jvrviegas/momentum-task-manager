// Package calendar provides the deliberately narrow, read-only calendar
// source used by daily planning. It reads local iCalendar files and never
// writes, syncs, or persists event content.
package calendar

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Event is the planning-safe event projection. Description/private attendee
// data is not retained beyond the current command result.
type Event struct {
	UID         string
	Summary     string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Transparent bool
	Source      string
}

type Interval struct {
	Start time.Time
	End   time.Time
}

type LoadResult struct {
	Events     []Event
	LoadedAt   time.Time
	Stale      bool
	SourcePath []string
	Err        error
}

type Options struct {
	Paths              []string
	IncludeAllDay      bool
	IncludeTransparent bool
	WorkingStart       string
	WorkingEnd         string
	StaleAfter         time.Duration
	Now                time.Time
}

// Load reads all configured local files for the day containing now. Missing
// or malformed files are returned as an error; callers may continue planning
// with calendar availability marked false.
func Load(ctx context.Context, options Options) LoadResult {
	if ctx == nil {
		ctx = context.Background()
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	result := LoadResult{LoadedAt: now, SourcePath: append([]string(nil), options.Paths...)}
	all := make([]Event, 0)
	for _, path := range options.Paths {
		select {
		case <-ctx.Done():
			result.Err = ctx.Err()
			return result
		default:
		}
		info, err := os.Stat(path)
		if err != nil {
			result.Err = fmt.Errorf("read calendar %s: %w", path, err)
			return result
		}
		if options.StaleAfter > 0 && now.Sub(info.ModTime()) > options.StaleAfter {
			result.Stale = true
		}
		data, err := os.ReadFile(path)
		if err != nil {
			result.Err = fmt.Errorf("read calendar %s: %w", path, err)
			return result
		}
		events, err := parseICS(data, now.Location(), path)
		if err != nil {
			result.Err = fmt.Errorf("parse calendar %s: %w", path, err)
			return result
		}
		all = append(all, events...)
	}
	result.Events = relevantEvents(all, now, options.IncludeTransparent, options.IncludeAllDay)
	return result
}

// BusyIntervals clips events to configured working hours, ignoring transparent
// entries by default. Use BusyIntervalsWithOptions when transparency is
// configured as busy.
func BusyIntervals(events []Event, day time.Time, workingStart, workingEnd string, includeAllDay bool) ([]Interval, error) {
	return BusyIntervalsWithOptions(events, day, workingStart, workingEnd, includeAllDay, false)
}

func BusyIntervalsWithOptions(events []Event, day time.Time, workingStart, workingEnd string, includeAllDay, includeTransparent bool) ([]Interval, error) {
	if day.IsZero() {
		day = time.Now()
	}
	startOffset, endOffset, err := parseWorkingHours(workingStart, workingEnd)
	if err != nil {
		return nil, err
	}
	loc := day.Location()
	date := time.Date(day.In(loc).Year(), day.In(loc).Month(), day.In(loc).Day(), 0, 0, 0, 0, loc)
	windowStart := date.Add(startOffset)
	windowEnd := date.Add(endOffset)
	intervals := make([]Interval, 0, len(events))
	for _, event := range events {
		if event.Transparent && !includeTransparent {
			continue
		}
		if event.AllDay {
			if !includeAllDay {
				continue
			}
			intervals = append(intervals, Interval{Start: windowStart, End: windowEnd})
			continue
		}
		start := event.Start.In(loc)
		end := event.End.In(loc)
		if end.Before(start) || end.Equal(start) || !end.After(windowStart) || !start.Before(windowEnd) {
			continue
		}
		if start.Before(windowStart) {
			start = windowStart
		}
		if end.After(windowEnd) {
			end = windowEnd
		}
		if end.After(start) {
			intervals = append(intervals, Interval{Start: start, End: end})
		}
	}
	return MergeIntervals(intervals), nil
}

func BusyMinutes(intervals []Interval) int {
	minutes := 0
	for _, interval := range intervals {
		if interval.End.After(interval.Start) {
			minutes += int(interval.End.Sub(interval.Start) / time.Minute)
		}
	}
	return minutes
}

func MergeIntervals(values []Interval) []Interval {
	intervals := append([]Interval(nil), values...)
	for index := range intervals {
		if intervals[index].End.Before(intervals[index].Start) {
			intervals[index].Start, intervals[index].End = intervals[index].End, intervals[index].Start
		}
	}
	// Small insertion sort avoids importing a second representation and keeps
	// equal-start ordering deterministic for the small daily event set.
	for i := 1; i < len(intervals); i++ {
		value := intervals[i]
		j := i - 1
		for j >= 0 && intervals[j].Start.After(value.Start) {
			intervals[j+1] = intervals[j]
			j--
		}
		intervals[j+1] = value
	}
	merged := make([]Interval, 0, len(intervals))
	for _, interval := range intervals {
		if !interval.End.After(interval.Start) {
			continue
		}
		if len(merged) == 0 || interval.Start.After(merged[len(merged)-1].End) {
			merged = append(merged, interval)
			continue
		}
		if interval.End.After(merged[len(merged)-1].End) {
			merged[len(merged)-1].End = interval.End
		}
	}
	return merged
}

func relevantEvents(events []Event, now time.Time, includeTransparent, includeAllDay bool) []Event {
	loc := now.Location()
	start := dateOnly(now.In(loc))
	end := start.AddDate(0, 0, 1)
	result := make([]Event, 0)
	seen := make(map[string]struct{})
	for _, event := range events {
		if event.Transparent && !includeTransparent {
			continue
		}
		if event.AllDay {
			if !includeAllDay {
				continue
			}
			if event.End.After(start) && event.Start.Before(end) {
				appendUniqueEvent(&result, seen, event)
			}
			continue
		}
		if event.End.After(start) && event.Start.Before(end) {
			appendUniqueEvent(&result, seen, event)
		}
	}
	return result
}

func appendUniqueEvent(events *[]Event, seen map[string]struct{}, event Event) {
	key := event.UID + "|" + event.Start.UTC().Format(time.RFC3339Nano) + "|" + event.End.UTC().Format(time.RFC3339Nano)
	if key == "|" {
		key = event.Source + "|" + event.Summary + "|" + event.Start.UTC().Format(time.RFC3339Nano)
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*events = append(*events, event)
}

type icsProperty struct {
	Name   string
	Params map[string]string
	Value  string
}

func parseICS(data []byte, fallbackLocation *time.Location, source string) ([]Event, error) {
	lines := unfoldLines(data)
	events := make([]Event, 0)
	var current *rawEvent
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "BEGIN:VEVENT") {
			current = &rawEvent{Source: source, Values: make(map[string][]icsProperty)}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(line), "END:VEVENT") {
			if current != nil {
				parsed, err := current.events(fallbackLocation)
				if err != nil {
					return nil, err
				}
				events = append(events, parsed...)
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}
		property, ok := parseProperty(line)
		if ok {
			current.Values[property.Name] = append(current.Values[property.Name], property)
		}
	}
	if current != nil {
		return nil, fmt.Errorf("unterminated VEVENT")
	}
	return events, nil
}

type rawEvent struct {
	Source string
	Values map[string][]icsProperty
}

func (r *rawEvent) events(fallback *time.Location) ([]Event, error) {
	if r.Values == nil {
		return nil, nil
	}
	if cancelled := firstValue(r.Values, "STATUS"); strings.EqualFold(cancelled.Value, "CANCELLED") {
		return nil, nil
	}
	startProperty, ok := firstProperty(r.Values, "DTSTART")
	if !ok {
		return nil, fmt.Errorf("VEVENT is missing DTSTART")
	}
	start, allDay, err := parseICSDate(startProperty, fallback)
	if err != nil {
		return nil, err
	}
	end := time.Time{}
	if endProperty, ok := firstProperty(r.Values, "DTEND"); ok {
		end, _, err = parseICSDate(endProperty, start.Location())
		if err != nil {
			return nil, err
		}
	} else if duration := firstValue(r.Values, "DURATION"); duration.Value != "" {
		end, err = start.Add(parseICSDuration(duration.Value)), nil
	} else if allDay {
		end = start.AddDate(0, 0, 1)
	} else {
		end = start.Add(time.Hour)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("VEVENT has non-positive duration")
	}
	uid := firstValue(r.Values, "UID").Value
	summary := firstValue(r.Values, "SUMMARY").Value
	transparent := strings.EqualFold(firstValue(r.Values, "TRANSP").Value, "TRANSPARENT")
	base := Event{UID: uid, Summary: summary, Start: start, End: end, AllDay: allDay, Transparent: transparent, Source: r.Source}
	rule := firstValue(r.Values, "RRULE").Value
	if strings.TrimSpace(rule) == "" {
		return []Event{base}, nil
	}
	return expandRecurring(base, rule)
}

func expandRecurring(base Event, rule string) ([]Event, error) {
	parts := make(map[string]string)
	for _, part := range strings.Split(rule, ";") {
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) == 2 {
			parts[strings.ToUpper(pieces[0])] = strings.ToUpper(pieces[1])
		}
	}
	freq := parts["FREQ"]
	if freq == "" {
		return []Event{base}, nil
	}
	interval := 1
	if value := parts["INTERVAL"]; value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("invalid RRULE interval %q", value)
		}
		interval = parsed
	}
	count := 0
	if value := parts["COUNT"]; value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("invalid RRULE count %q", value)
		}
		count = parsed
	}
	until := time.Time{}
	if value := parts["UNTIL"]; value != "" {
		property := icsProperty{Value: value}
		until, _, _ = parseICSDate(property, base.Start.Location())
	}
	wantedDays := make([]int, 0)
	if value := parts["BYDAY"]; value != "" {
		for _, item := range strings.Split(value, ",") {
			if weekday, ok := icsWeekday(item); ok {
				wantedDays = append(wantedDays, int(weekday))
			}
		}
	}
	// Expand only a bounded window around the current event. The caller filters
	// the resulting day; 370 iterations cover yearly and monthly rules without
	// allowing malformed calendars to create an unbounded operation.
	result := make([]Event, 0)
	current := base
	anchorWeek := monday(current.Start)
	for occurrence := 0; occurrence < 370; occurrence++ {
		if count > 0 && occurrence >= count {
			break
		}
		if !until.IsZero() && current.Start.After(until) {
			break
		}
		result = append(result, current)
		next := current.Start
		switch freq {
		case "DAILY":
			next = next.AddDate(0, 0, interval)
		case "WEEKLY":
			if len(wantedDays) > 0 {
				found := false
				for offset := 1; offset <= 7*interval+7; offset++ {
					candidate := current.Start.AddDate(0, 0, offset)
					weekDistance := calendarWeekDistance(candidate, anchorWeek)
					if weekDistance < 0 || weekDistance%interval != 0 {
						continue
					}
					for _, wantedDay := range wantedDays {
						if int(candidate.Weekday()) == wantedDay {
							next = candidate
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					return result, nil
				}
			} else {
				next = next.AddDate(0, 0, 7*interval)
			}
		case "MONTHLY":
			next = next.AddDate(0, interval, 0)
		case "YEARLY":
			next = next.AddDate(interval, 0, 0)
		default:
			return []Event{base}, nil
		}
		delta := current.End.Sub(current.Start)
		current.Start = next
		current.End = next.Add(delta)
	}
	return result, nil
}

func parseProperty(line string) (icsProperty, bool) {
	pieces := strings.SplitN(line, ":", 2)
	if len(pieces) != 2 {
		return icsProperty{}, false
	}
	header := strings.Split(pieces[0], ";")
	property := icsProperty{Name: strings.ToUpper(header[0]), Value: strings.TrimSpace(pieces[1]), Params: map[string]string{}}
	for _, param := range header[1:] {
		value := strings.SplitN(param, "=", 2)
		if len(value) == 2 {
			property.Params[strings.ToUpper(value[0])] = strings.Trim(value[1], "\"")
		}
	}
	return property, true
}

func firstProperty(values map[string][]icsProperty, name string) (icsProperty, bool) {
	items := values[strings.ToUpper(name)]
	if len(items) == 0 {
		return icsProperty{}, false
	}
	return items[0], true
}

func firstValue(values map[string][]icsProperty, name string) icsProperty {
	value, _ := firstProperty(values, name)
	return value
}

func parseICSDate(property icsProperty, fallback *time.Location) (time.Time, bool, error) {
	value := strings.TrimSpace(property.Value)
	location := fallback
	if location == nil {
		location = time.Local
	}
	if tzid := property.Params["TZID"]; tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			location = loaded
		}
	}
	if len(value) == 8 && !strings.Contains(value, "T") {
		parsed, err := time.ParseInLocation("20060102", value, location)
		return parsed, true, err
	}
	if strings.HasSuffix(value, "Z") {
		parsed, err := time.Parse("20060102T150405Z", value)
		return parsed, false, err
	}
	for _, layout := range []string{"20060102T150405", "20060102T1504"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("invalid ICS date %q", value)
}

func parseICSDuration(value string) time.Duration {
	value = strings.TrimSpace(strings.ToUpper(value))
	sign := 1
	if strings.HasPrefix(value, "-") {
		sign = -1
		value = strings.TrimPrefix(value, "-")
	}
	value = strings.TrimPrefix(value, "+")
	if !strings.HasPrefix(value, "P") {
		return 0
	}
	value = strings.TrimPrefix(value, "P")
	date, clock := value, ""
	if index := strings.IndexByte(value, 'T'); index >= 0 {
		date, clock = value[:index], value[index+1:]
	}
	read := func(text string, unit byte) int64 {
		index := strings.IndexByte(text, unit)
		if index < 0 {
			return 0
		}
		start := index - 1
		for start >= 0 && text[start] >= '0' && text[start] <= '9' {
			start--
		}
		value, _ := strconv.ParseInt(text[start+1:index], 10, 64)
		return value
	}
	seconds := read(date, 'D') * 86400
	seconds += read(clock, 'H') * 3600
	seconds += read(clock, 'M') * 60
	seconds += read(clock, 'S')
	return time.Duration(int64(sign)*seconds) * time.Second
}

func parseWorkingHours(start, end string) (time.Duration, time.Duration, error) {
	if start == "" {
		start = "09:00"
	}
	if end == "" {
		end = "17:00"
	}
	parse := func(value string) (time.Duration, error) {
		parsed, err := time.Parse("15:04", value)
		if err != nil {
			return 0, err
		}
		return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
	}
	left, err := parse(start)
	if err != nil {
		return 0, 0, fmt.Errorf("working start must use HH:MM: %w", err)
	}
	right, err := parse(end)
	if err != nil {
		return 0, 0, fmt.Errorf("working end must use HH:MM: %w", err)
	}
	if right <= left {
		return 0, 0, fmt.Errorf("working end must be after working start")
	}
	return left, right, nil
}

func icsWeekday(value string) (time.Weekday, bool) {
	value = strings.TrimSpace(value)
	if len(value) > 2 {
		value = value[len(value)-2:]
	}
	weekdays := map[string]time.Weekday{"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday}
	weekday, ok := weekdays[value]
	return weekday, ok
}

func unfoldLines(data []byte) []string {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lines := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
		} else {
			lines = append(lines, line)
		}
	}
	return lines
}

func dateOnly(value time.Time) time.Time {
	value = value.In(value.Location())
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func monday(value time.Time) time.Time {
	value = dateOnly(value)
	delta := (int(value.Weekday()) + 6) % 7
	return value.AddDate(0, 0, -delta)
}

func calendarWeekDistance(value, anchor time.Time) int {
	value = dateOnly(value)
	anchor = dateOnly(anchor)
	valueUTC := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	anchorUTC := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	return int(valueUTC.Sub(anchorUTC) / (7 * 24 * time.Hour))
}
