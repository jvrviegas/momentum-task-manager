package domain

import (
	"sort"
	"strings"
	"time"
)

// ChangeKind describes how a supported Taskwarrior field should be modified.
type ChangeKind uint8

const (
	Unchanged ChangeKind = iota
	Set
	Clear
)

// FieldChange distinguishes an unchanged field from an explicit clear.
type FieldChange struct {
	Kind  ChangeKind
	Value string
}

func setChange(value string) FieldChange {
	return FieldChange{Kind: Set, Value: value}
}

func clearChange() FieldChange { return FieldChange{Kind: Clear} }

// Empty reports whether this individual field carries no value change.
func (c FieldChange) Empty() bool {
	return c.Kind == Unchanged
}

// EstimateChange is the typed equivalent of FieldChange for the optional
// estimate. Value is populated only for Set; Clear is represented by a nil
// Value and an explicit Kind.
type EstimateChange struct {
	Kind  ChangeKind
	Value *Estimate
}

// Empty reports whether the estimate carries no value change.
func (c EstimateChange) Empty() bool {
	return c.Kind == Unchanged
}

// TagChange is a deterministic set difference for Taskwarrior +tag/-tag args.
type TagChange struct {
	Changed bool
	Add     []string
	Remove  []string
}

// TaskDiff contains only the fields Momentum is allowed to edit.
type TaskDiff struct {
	Description FieldChange
	Project     FieldChange
	Priority    FieldChange
	Due         FieldChange
	Scheduled   FieldChange
	Recurrence  FieldChange
	Estimate    EstimateChange
	Tags        TagChange
}

// Empty reports whether a modify command would have no supported changes.
func (d TaskDiff) Empty() bool {
	return d.Description.Kind == Unchanged &&
		d.Project.Kind == Unchanged &&
		d.Priority.Kind == Unchanged &&
		d.Due.Kind == Unchanged &&
		d.Scheduled.Kind == Unchanged &&
		d.Recurrence.Kind == Unchanged &&
		d.Estimate.Empty() &&
		!d.Tags.Changed
}

// EditSnapshot is the editable projection of a Task.
type EditSnapshot struct {
	Description string
	Project     string
	Priority    string
	Due         string
	Scheduled   string
	Recurrence  string
	Estimate    *Estimate
	Tags        []string
}

// Snapshot returns the supported editable fields without including raw or
// unsupported Taskwarrior properties.
func Snapshot(task Task) EditSnapshot {
	return EditSnapshot{
		Description: task.Description,
		Project:     task.Project,
		Priority:    task.Priority,
		Due:         snapshotDate(task.Due, task.DueRaw),
		Scheduled:   snapshotDate(task.Scheduled, task.ScheduledRaw),
		Recurrence:  task.Recurrence,
		Estimate:    cloneEstimate(task.Estimate),
		Tags:        uniqueTags(task.Tags),
	}
}

func snapshotDate(value *time.Time, raw string) string {
	if raw != "" {
		return raw
	}
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

// Diff computes the minimal supported-field changes between two snapshots.
func Diff(before, after EditSnapshot) TaskDiff {
	return TaskDiff{
		Description: scalarChange(before.Description, after.Description),
		Project:     scalarChange(before.Project, after.Project),
		Priority:    scalarChange(before.Priority, after.Priority),
		Due:         scalarChange(before.Due, after.Due),
		Scheduled:   scalarChange(before.Scheduled, after.Scheduled),
		Recurrence:  scalarChange(before.Recurrence, after.Recurrence),
		Estimate:    estimateChange(before.Estimate, after.Estimate),
		Tags:        tagDiff(before.Tags, after.Tags),
	}
}

// GenerateDiff is an alias that makes the operation explicit at call sites.
func GenerateDiff(before, after EditSnapshot) TaskDiff { return Diff(before, after) }

func scalarChange(before, after string) FieldChange {
	if before == after {
		return FieldChange{}
	}
	if after == "" {
		return clearChange()
	}
	return setChange(after)
}

func estimateChange(before, after *Estimate) EstimateChange {
	if sameEstimate(before, after) {
		return EstimateChange{}
	}
	if after == nil {
		return EstimateChange{Kind: Clear}
	}
	return EstimateChange{Kind: Set, Value: cloneEstimate(after)}
}

func sameEstimate(left, right *Estimate) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Minutes == right.Minutes
}

func cloneEstimate(value *Estimate) *Estimate {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func tagDiff(before, after []string) TagChange {
	beforeSet := make(map[string]struct{}, len(before))
	afterSet := make(map[string]struct{}, len(after))
	for _, tag := range uniqueTags(before) {
		beforeSet[tag] = struct{}{}
	}
	for _, tag := range uniqueTags(after) {
		afterSet[tag] = struct{}{}
	}
	change := TagChange{}
	for tag := range afterSet {
		if _, exists := beforeSet[tag]; !exists {
			change.Add = append(change.Add, tag)
		}
	}
	for tag := range beforeSet {
		if _, exists := afterSet[tag]; !exists {
			change.Remove = append(change.Remove, tag)
		}
	}
	sort.Strings(change.Add)
	sort.Strings(change.Remove)
	change.Changed = len(change.Add) > 0 || len(change.Remove) > 0
	return change
}

func uniqueTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}
