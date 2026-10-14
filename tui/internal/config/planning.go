package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// PlanningConfig owns only the local daily-planning policy. Task selections
// themselves remain Taskwarrior tags, not Momentum persistence.
type PlanningConfig struct {
	Enabled         bool              `toml:"enabled"`
	DailyCapacity   time.Duration     `toml:"daily_capacity"`
	Buffer          time.Duration     `toml:"buffer"`
	WeekdayCapacity map[string]string `toml:"weekday_capacity"`
}

// CalendarConfig selects the first supported calendar source: local ICS
// files. Fetching/synchronizing those files is deliberately outside Momentum;
// the app only reads them when planning is opened.
type CalendarConfig struct {
	Enabled            bool          `toml:"enabled"`
	Paths              []string      `toml:"paths"`
	IncludeAllDay      bool          `toml:"include_all_day"`
	IncludeTransparent bool          `toml:"include_transparent"`
	WorkingStart       string        `toml:"working_start"`
	WorkingEnd         string        `toml:"working_end"`
	StaleAfter         time.Duration `toml:"stale_after"`
}

func defaultPlanningConfig() PlanningConfig {
	return PlanningConfig{Enabled: true, DailyCapacity: 8 * time.Hour, Buffer: time.Hour}
}

func defaultCalendarConfig() CalendarConfig {
	return CalendarConfig{
		WorkingStart: "09:00",
		WorkingEnd:   "17:00",
		StaleAfter:   24 * time.Hour,
	}
}

func (c PlanningConfig) Validate() error {
	if c.DailyCapacity <= 0 {
		return fmt.Errorf("config key %q must be a positive duration", "planning.daily_capacity")
	}
	if c.Buffer < 0 {
		return fmt.Errorf("config key %q must be zero or a positive duration", "planning.buffer")
	}
	for day, value := range c.WeekdayCapacity {
		if _, ok := weekdayName(day); !ok {
			return fmt.Errorf("config key %q has unknown weekday %q", "planning.weekday_capacity", day)
		}
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("config key %q for %s must be a positive duration", "planning.weekday_capacity", day)
		}
	}
	return nil
}

func (c PlanningConfig) CapacityFor(day time.Weekday) time.Duration {
	if value, ok := c.WeekdayCapacity[strings.ToLower(day.String())]; ok {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return c.DailyCapacity
}

func (c CalendarConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("config key %q requires at least one local ICS path", "calendar.paths")
	}
	for _, value := range c.Paths {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("config key %q cannot contain an empty path", "calendar.paths")
		}
		if strings.Contains(value, "://") {
			return fmt.Errorf("config key %q accepts local ICS paths only; download subscribed calendars outside Momentum", "calendar.paths")
		}
	}
	if _, err := time.Parse("15:04", c.WorkingStart); err != nil {
		return fmt.Errorf("config key %q must use HH:MM", "calendar.working_start")
	}
	if _, err := time.Parse("15:04", c.WorkingEnd); err != nil {
		return fmt.Errorf("config key %q must use HH:MM", "calendar.working_end")
	}
	start, _ := time.Parse("15:04", c.WorkingStart)
	end, _ := time.Parse("15:04", c.WorkingEnd)
	if !end.After(start) {
		return fmt.Errorf("config keys %q and %q must define a same-day interval", "calendar.working_start", "calendar.working_end")
	}
	if c.StaleAfter < 0 {
		return fmt.Errorf("config key %q must be zero or a positive duration", "calendar.stale_after")
	}
	return nil
}

func (c CalendarConfig) WorkingHours() (time.Duration, time.Duration) {
	start, _ := time.Parse("15:04", c.WorkingStart)
	end, _ := time.Parse("15:04", c.WorkingEnd)
	return time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute,
		time.Duration(end.Hour())*time.Hour + time.Duration(end.Minute())*time.Minute
}

func (c CalendarConfig) PathsFor(home string) []string {
	paths := make([]string, 0, len(c.Paths))
	for _, value := range c.Paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if value == "~" {
			value = home
		} else if strings.HasPrefix(value, "~/") {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
		paths = append(paths, value)
	}
	return paths
}

func weekdayName(value string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return 0, false
	}
}
