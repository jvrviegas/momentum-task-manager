package config

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tomledit "github.com/smm-h/go-toml-edit"
)

// FieldError is a value a Settings tab refused. Field names the field that
// holds it, and Message says what to enter or do instead.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

const lengthHint = "Enter a length of time, such as 8h or 7h30m."

// ParseLength reads a length of time as a person types it, such as 8h, 7h30m
// or 45m, with the config's own parser.
func ParseLength(text string) (time.Duration, error) {
	return time.ParseDuration(strings.TrimSpace(text))
}

// FormatLength is the shortest way to type a length: 8h, 7h30m, 45m.
func FormatLength(d time.Duration) string {
	if d%time.Minute != 0 {
		return d.String()
	}
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case hours == 0 && minutes == 0:
		return "0m"
	case minutes == 0:
		return fmt.Sprintf("%dh", hours)
	case hours == 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
}

// RevisionID names a version of config.toml, so a Settings tab can say which
// one it read. A file that does not exist yet has its own.
func RevisionID(revision ProjectCatalogRevision) string {
	if !revision.Exists {
		return "missing"
	}
	return hex.EncodeToString(revision.Digest[:])
}

// PlanningPatch holds the [planning] keys a Settings tab changes. A nil field
// is left as it is.
type PlanningPatch struct {
	Enabled       *bool
	DailyCapacity *string
	Buffer        *string
	// WeekdayCapacity maps a weekday's name to the length typed for it. An
	// empty length removes the weekday's own capacity, so it uses the daily one.
	WeekdayCapacity map[string]string
}

// Check refuses what the config would refuse, with a message that says what
// to type instead.
func (p PlanningPatch) Check() *FieldError {
	if p.DailyCapacity != nil {
		length, err := ParseLength(*p.DailyCapacity)
		switch {
		case err != nil:
			return &FieldError{Field: "dailyCapacity", Message: lengthHint}
		case length <= 0:
			return &FieldError{Field: "dailyCapacity", Message: "Daily capacity must be longer than zero."}
		}
	}
	if p.Buffer != nil {
		length, err := ParseLength(*p.Buffer)
		switch {
		case err != nil:
			return &FieldError{Field: "buffer", Message: lengthHint}
		case length < 0:
			return &FieldError{Field: "buffer", Message: "Buffer can’t be negative."}
		}
	}
	for _, day := range sortedKeys(p.WeekdayCapacity) {
		field := "weekdayCapacity." + strings.ToLower(day)
		if _, ok := weekdayName(day); !ok {
			return &FieldError{Field: field, Message: "Use a weekday name, such as monday."}
		}
		value := strings.TrimSpace(p.WeekdayCapacity[day])
		if value == "" {
			continue
		}
		length, err := ParseLength(value)
		switch {
		case err != nil:
			return &FieldError{Field: field, Message: lengthHint}
		case length <= 0:
			return &FieldError{Field: field, Message: "A day must be longer than zero. Leave it empty to use the daily capacity."}
		}
	}
	return nil
}

// SavePlanning sets the patch's [planning] keys with the same conflict checks
// and atomic replacement as Save, and keeps every comment and unrelated key.
func (s *FileProjectCatalogStore) SavePlanning(ctx context.Context, snapshot ProjectCatalogSnapshot, patch PlanningPatch) (ProjectCatalogSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	if fieldErr := patch.Check(); fieldErr != nil {
		return ProjectCatalogSnapshot{}, fieldErr
	}
	return s.save(ctx, snapshot, "planning", func(doc *tomledit.Document) error {
		if patch.Enabled != nil {
			if err := doc.SetCreate("planning.enabled", *patch.Enabled); err != nil {
				return err
			}
		}
		if patch.DailyCapacity != nil {
			if err := doc.SetCreate("planning.daily_capacity", strings.TrimSpace(*patch.DailyCapacity)); err != nil {
				return err
			}
		}
		if patch.Buffer != nil {
			if err := doc.SetCreate("planning.buffer", strings.TrimSpace(*patch.Buffer)); err != nil {
				return err
			}
		}
		for _, day := range sortedKeys(patch.WeekdayCapacity) {
			key := "planning.weekday_capacity." + strings.ToLower(day)
			if value := strings.TrimSpace(patch.WeekdayCapacity[day]); value == "" {
				if err := doc.Delete(key); err != nil {
					return err
				}
			} else if err := doc.SetCreate(key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

// CalendarPatch holds the [calendar] keys a Settings tab changes. A nil field
// is left as it is.
type CalendarPatch struct {
	Enabled *bool
	// Paths is the whole list of calendar files.
	Paths *[]string
}

// Normalized tidies the file list: a path under home is kept as ~/..., an
// empty one and a repeat are dropped.
func (p CalendarPatch) Normalized(home string) CalendarPatch {
	if p.Paths == nil {
		return p
	}
	seen := map[string]bool{}
	paths := []string{}
	for _, value := range *p.Paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		expanded := ExpandHome(value, home)
		if seen[expanded] {
			continue
		}
		seen[expanded] = true
		if home != "" && strings.HasPrefix(expanded, home+string(filepath.Separator)) {
			value = "~/" + filepath.ToSlash(strings.TrimPrefix(expanded, home+string(filepath.Separator)))
		}
		paths = append(paths, value)
	}
	p.Paths = &paths
	return p
}

// Check refuses what the config would refuse, with a message that says what
// to do instead. current is the calendar settings before the change.
func (p CalendarPatch) Check(current CalendarConfig) *FieldError {
	enabled, paths := current.Enabled, current.Paths
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	if p.Paths != nil {
		paths = *p.Paths
	}
	for _, value := range paths {
		if strings.Contains(value, "://") {
			return &FieldError{Field: "paths", Message: "Choose a file on this Mac. Download a subscribed calendar first, then add the file."}
		}
	}
	if enabled && len(paths) == 0 {
		if p.Enabled == nil {
			return &FieldError{Field: "paths", Message: "Turn off Use calendar files before removing the last one."}
		}
		return &FieldError{Field: "enabled", Message: "Add a calendar file first."}
	}
	return nil
}

// SaveCalendar sets the patch's [calendar] keys, as SavePlanning does.
func (s *FileProjectCatalogStore) SaveCalendar(ctx context.Context, snapshot ProjectCatalogSnapshot, patch CalendarPatch) (ProjectCatalogSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	return s.save(ctx, snapshot, "calendar", func(doc *tomledit.Document) error {
		if patch.Enabled != nil {
			if err := doc.SetCreate("calendar.enabled", *patch.Enabled); err != nil {
				return err
			}
		}
		if patch.Paths != nil {
			values := make([]any, len(*patch.Paths))
			for i, value := range *patch.Paths {
				values[i] = value
			}
			if err := doc.SetCreate("calendar.paths", values); err != nil {
				return err
			}
		}
		return nil
	})
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
