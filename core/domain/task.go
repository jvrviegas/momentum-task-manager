package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Annotation is a Taskwarrior annotation attached to a task.
type Annotation struct {
	Description string     `json:"description"`
	Entry       *time.Time `json:"-"`
	EntryRaw    string     `json:"-"`
}

// Task is the supported, export-facing representation of a Taskwarrior task.
// Date pointers preserve the difference between an absent date and a zero time.
type Task struct {
	UUID        string
	ID          int
	Description string
	Status      string
	Project     string
	Priority    string
	Estimate    *Estimate
	// EstimateWarning is non-empty when Taskwarrior exported an estimate value
	// that cannot be represented as positive whole minutes. The task remains
	// usable and the original JSON stays in RawFields for read-only details.
	EstimateWarning string
	Entry           *time.Time
	End             *time.Time
	Modified        *time.Time
	Due             *time.Time
	Scheduled       *time.Time
	Start           *time.Time
	Wait            *time.Time
	Until           *time.Time
	EntryRaw        string
	EndRaw          string
	ModifiedRaw     string
	DueRaw          string
	ScheduledRaw    string
	StartRaw        string
	WaitRaw         string
	UntilRaw        string
	Tags            []string
	Annotations     []Annotation
	Dependencies    []string
	Recurrence      string
	Parent          string
	Mask            string
	MaskIndex       *int
	RecurrenceType  string
	Urgency         float64
	RawFields       map[string]json.RawMessage
}

// IsPending reports whether Taskwarrior considers the task pending.
func (t Task) IsPending() bool {
	return strings.EqualFold(t.Status, "pending")
}

// UnmarshalJSON decodes Taskwarrior's export shape while retaining every raw
// property for read-only details and diagnostics.
func (t *Task) UnmarshalJSON(data []byte) error {
	type wire struct {
		UUID           string           `json:"uuid"`
		ID             int              `json:"id"`
		Description    string           `json:"description"`
		Status         string           `json:"status"`
		Project        string           `json:"project"`
		Priority       string           `json:"priority"`
		Entry          json.RawMessage  `json:"entry"`
		End            json.RawMessage  `json:"end"`
		Modified       json.RawMessage  `json:"modified"`
		Due            json.RawMessage  `json:"due"`
		Scheduled      json.RawMessage  `json:"scheduled"`
		Start          json.RawMessage  `json:"start"`
		Wait           json.RawMessage  `json:"wait"`
		Until          json.RawMessage  `json:"until"`
		Tags           []string         `json:"tags"`
		Annotations    []AnnotationWire `json:"annotations"`
		Dependencies   []string         `json:"depends"`
		Recurrence     string           `json:"recur"`
		Parent         string           `json:"parent"`
		Mask           string           `json:"mask"`
		MaskIndex      *int             `json:"imask"`
		RecurrenceType string           `json:"rtype"`
		Urgency        float64          `json:"urgency"`
	}

	var value wire
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode task: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("retain task fields: %w", err)
	}

	estimate, estimateWarning := decodeTaskwarriorEstimate(raw["estimate"])
	entry, entryRaw, err := optionalTime(value.Entry, "entry")
	if err != nil {
		return err
	}
	end, endRaw, err := optionalTime(value.End, "end")
	if err != nil {
		return err
	}
	modified, modifiedRaw, err := optionalTime(value.Modified, "modified")
	if err != nil {
		return err
	}
	due, dueRaw, err := optionalTime(value.Due, "due")
	if err != nil {
		return err
	}
	scheduled, scheduledRaw, err := optionalTime(value.Scheduled, "scheduled")
	if err != nil {
		return err
	}
	start, startRaw, err := optionalTime(value.Start, "start")
	if err != nil {
		return err
	}
	wait, waitRaw, err := optionalTime(value.Wait, "wait")
	if err != nil {
		return err
	}
	until, untilRaw, err := optionalTime(value.Until, "until")
	if err != nil {
		return err
	}

	annotations := make([]Annotation, 0, len(value.Annotations))
	for _, annotation := range value.Annotations {
		parsed, rawEntry, err := optionalTime(annotation.Entry, "annotation entry")
		if err != nil {
			return err
		}
		annotations = append(annotations, Annotation{
			Description: annotation.Description,
			Entry:       parsed,
			EntryRaw:    rawEntry,
		})
	}

	*t = Task{
		UUID:            value.UUID,
		ID:              value.ID,
		Description:     value.Description,
		Status:          value.Status,
		Project:         value.Project,
		Priority:        value.Priority,
		Estimate:        estimate,
		EstimateWarning: estimateWarning,
		Entry:           entry,
		End:             end,
		Modified:        modified,
		Due:             due,
		Scheduled:       scheduled,
		Start:           start,
		Wait:            wait,
		Until:           until,
		EntryRaw:        entryRaw,
		EndRaw:          endRaw,
		ModifiedRaw:     modifiedRaw,
		DueRaw:          dueRaw,
		ScheduledRaw:    scheduledRaw,
		StartRaw:        startRaw,
		WaitRaw:         waitRaw,
		UntilRaw:        untilRaw,
		Tags:            append([]string(nil), value.Tags...),
		Dependencies:    append([]string(nil), value.Dependencies...),
		Recurrence:      value.Recurrence,
		Parent:          value.Parent,
		Mask:            value.Mask,
		MaskIndex:       value.MaskIndex,
		RecurrenceType:  value.RecurrenceType,
		Urgency:         value.Urgency,
		RawFields:       raw,
	}
	for _, annotation := range annotations {
		t.Annotations = append(t.Annotations, annotation)
	}
	return nil
}

type AnnotationWire struct {
	Description string          `json:"description"`
	Entry       json.RawMessage `json:"entry"`
}

func decodeTaskwarriorEstimate(raw json.RawMessage) (*Estimate, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, "exported estimate is not a string"
	}
	if strings.TrimSpace(text) == "" {
		return nil, "exported estimate is empty"
	}
	estimate, err := ParseTaskwarriorEstimate(text)
	if err != nil {
		return nil, "unsupported exported estimate: " + err.Error()
	}
	return &estimate, ""
}

func optionalTime(raw json.RawMessage, field string) (*time.Time, string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, "", fmt.Errorf("decode %s date: %w", field, err)
	}
	if text == "" {
		return nil, "", nil
	}
	parsed, err := parseTaskwarriorTime(text)
	if err != nil {
		return nil, text, fmt.Errorf("decode %s date %q: %w", field, text, err)
	}
	return &parsed, text, nil
}

// ParseTaskwarriorTime parses the timestamp formats emitted by Taskwarrior 3.x.
func ParseTaskwarriorTime(value string) (time.Time, error) {
	return parseTaskwarriorTime(value)
}

func parseTaskwarriorTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339Nano,
		"20060102T150405Z07:00",
		"20060102T150405Z",
		"20060102T150405",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02",
		"20060102",
	}
	var lastErr error
	for _, layout := range layouts {
		var parsed time.Time
		var err error
		if strings.Contains(layout, "Z07") || strings.HasSuffix(layout, "Z") {
			parsed, err = time.Parse(layout, value)
		} else {
			parsed, err = time.ParseInLocation(layout, value, time.Local)
		}
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

// DateIn returns a task date in loc, or false when the field is absent.
func DateIn(value *time.Time, loc *time.Location) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}
	return value.In(loc), true
}

// Raw returns an exported property exactly as it appeared in the JSON object.
func (t Task) Raw(name string) (json.RawMessage, bool) {
	value, ok := t.RawFields[name]
	return value, ok
}
