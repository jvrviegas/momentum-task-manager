package domain

import (
	"sort"
	"strings"
	"time"
)

// DailyPlanTagPrefix namespaces Momentum's daily commitment marker inside
// Taskwarrior's ordinary tag set. Tags are durable, inspectable, syncable task
// data; Momentum owns no parallel plan database.
const DailyPlanTagPrefix = "momentum-plan-"

type PlanItemGroup string

const (
	PlanObligation PlanItemGroup = "obligation"
	PlanRollover   PlanItemGroup = "rollover"
	PlanCandidate  PlanItemGroup = "candidate"
)

// PlanItem is the read-only planning projection rendered by the ritual.
type PlanItem struct {
	Task     Task
	Group    PlanItemGroup
	Selected bool
	Fixed    bool
}

// PlanningConstraints are the inputs to capacity arithmetic. CalendarMinutes
// is zero when calendar data is disabled or unavailable; callers use the
// availability flags to avoid presenting that as a successful calendar read.
type PlanningConstraints struct {
	CapacityMinutes   int
	BufferMinutes     int
	CalendarMinutes   int
	MeetingCount      int
	CalendarAvailable bool
	CalendarStale     bool
}

// CapacitySummary keeps fixed obligations separate from voluntary selected
// work. Unestimated tasks are counted, never treated as zero effort.
type CapacitySummary struct {
	ConfiguredMinutes   int
	FixedMinutes        int
	CalendarMinutes     int
	BufferMinutes       int
	AvailableMinutes    int
	SelectedMinutes     int
	UnestimatedSelected int
	UnestimatedFixed    int
	MeetingCount        int
	CalendarAvailable   bool
	CalendarStale       bool
	OverCapacity        bool
}

// DailyPlan is an ephemeral projection. Its durable result is represented by
// DailyPlanTagFor on the selected Taskwarrior tasks.
type DailyPlan struct {
	Date        time.Time
	Obligations []PlanItem
	Rollover    []PlanItem
	Candidates  []PlanItem
	Selected    map[string]bool
	Capacity    CapacitySummary
}

// DailyPlanTagFor returns the exact marker for a local calendar date.
func DailyPlanTagFor(value time.Time) string {
	value = value.In(locationOrLocal(value.Location()))
	return DailyPlanTagPrefix + value.Format("2006-01-02")
}

// PlannedDate returns the first Momentum daily-plan marker in task tags.
func PlannedDate(task Task) (time.Time, bool) {
	for _, tag := range task.Tags {
		if !strings.HasPrefix(tag, DailyPlanTagPrefix) {
			continue
		}
		value := strings.TrimPrefix(tag, DailyPlanTagPrefix)
		parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func IsPlannedFor(task Task, date time.Time) bool {
	planned, ok := PlannedDate(task)
	if !ok {
		return false
	}
	date = date.In(locationOrLocal(date.Location()))
	return planned.Year() == date.Year() && planned.Month() == date.Month() && planned.Day() == date.Day()
}

func IsPlanTag(tag string) bool { return strings.HasPrefix(tag, DailyPlanTagPrefix) }

func planTags(tags []string) []string {
	result := make([]string, 0)
	for _, tag := range tags {
		if IsPlanTag(tag) {
			result = append(result, tag)
		}
	}
	sort.Strings(result)
	return result
}

// BuildDailyPlan derives obligations, rollover, and candidates from one
// pending export. Obligations are never silently treated as voluntary work.
func BuildDailyPlan(tasks []Task, now time.Time, constraints PlanningConstraints) DailyPlan {
	if now.IsZero() {
		now = time.Now()
	}
	today := dateOnly(now.In(now.Location()))
	plan := DailyPlan{Date: today, Selected: make(map[string]bool)}
	for _, task := range tasks {
		if !task.IsPending() {
			continue
		}
		identity := taskIdentity(task)
		if group := ClassifyToday(task, now); group != "" {
			plan.Obligations = append(plan.Obligations, PlanItem{Task: task, Group: PlanObligation, Fixed: true, Selected: IsPlannedFor(task, today)})
			if IsPlannedFor(task, today) {
				plan.Selected[identity] = true
			}
			continue
		}
		if planned, ok := PlannedDate(task); ok && calendarDateBefore(planned, today) {
			plan.Rollover = append(plan.Rollover, PlanItem{Task: task, Group: PlanRollover, Selected: true})
			plan.Selected[identity] = true
			continue
		}
		selected := IsPlannedFor(task, today)
		plan.Candidates = append(plan.Candidates, PlanItem{Task: task, Group: PlanCandidate, Selected: selected})
		if selected {
			plan.Selected[identity] = true
		}
	}
	SortTasksByPlan(plan.Obligations)
	SortTasksByPlan(plan.Rollover)
	SortTasksByPlan(plan.Candidates)
	plan.Capacity = summarizeCapacity(plan, constraints)
	return plan
}

func SortTasksByPlan(items []PlanItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i].Task, items[j].Task
		if left.Urgency != right.Urgency {
			return left.Urgency > right.Urgency
		}
		if strings.ToLower(left.Project) != strings.ToLower(right.Project) {
			return strings.ToLower(left.Project) < strings.ToLower(right.Project)
		}
		if left.Entry != nil && right.Entry != nil && !left.Entry.Equal(*right.Entry) {
			return left.Entry.Before(*right.Entry)
		}
		return taskIdentity(left) < taskIdentity(right)
	})
}

func summarizeCapacity(plan DailyPlan, constraints PlanningConstraints) CapacitySummary {
	capacity := CapacitySummary{
		ConfiguredMinutes: maxInt(0, constraints.CapacityMinutes),
		BufferMinutes:     maxInt(0, constraints.BufferMinutes),
		CalendarMinutes:   maxInt(0, constraints.CalendarMinutes),
		MeetingCount:      maxInt(0, constraints.MeetingCount),
		CalendarAvailable: constraints.CalendarAvailable,
		CalendarStale:     constraints.CalendarStale,
	}
	for _, item := range plan.Obligations {
		if item.Task.Estimate == nil {
			capacity.UnestimatedFixed++
			continue
		}
		capacity.FixedMinutes += item.Task.Estimate.Minutes
	}
	for _, item := range plan.Rollover {
		if !item.Selected {
			continue
		}
		if item.Task.Estimate == nil {
			capacity.UnestimatedSelected++
			continue
		}
		capacity.SelectedMinutes += item.Task.Estimate.Minutes
	}
	for _, item := range plan.Candidates {
		if !item.Selected {
			continue
		}
		if item.Task.Estimate == nil {
			capacity.UnestimatedSelected++
			continue
		}
		capacity.SelectedMinutes += item.Task.Estimate.Minutes
	}
	capacity.AvailableMinutes = capacity.ConfiguredMinutes - capacity.FixedMinutes - capacity.CalendarMinutes - capacity.BufferMinutes
	capacity.OverCapacity = capacity.SelectedMinutes > capacity.AvailableMinutes
	return capacity
}

// RecalculateCapacity applies an updated calendar result without rebuilding
// the user's selection.
func (p *DailyPlan) RecalculateCapacity(constraints PlanningConstraints) {
	if p == nil {
		return
	}
	p.Capacity = summarizeCapacity(*p, constraints)
}

// PlanMutation is one idempotent UUID/tag change in a plan commit.
type PlanMutation struct {
	UUID string
	Diff TaskDiff
}

// BuildPlanMutations computes only tag changes needed to make the selected set
// match today's plan. Repeating it after a partial failure is safe.
func BuildPlanMutations(tasks []Task, selected map[string]bool, date time.Time) []PlanMutation {
	currentTag := DailyPlanTagFor(date)
	mutations := make([]PlanMutation, 0)
	for _, task := range tasks {
		if !task.IsPending() || task.UUID == "" {
			continue
		}
		want := selected[task.UUID]
		before := append([]string(nil), task.Tags...)
		after := make([]string, 0, len(before)+1)
		for _, tag := range before {
			if IsPlanTag(tag) {
				continue
			}
			after = append(after, tag)
		}
		if want {
			after = append(after, currentTag)
		}
		diff := tagDiff(before, after)
		if !diff.Changed {
			continue
		}
		mutations = append(mutations, PlanMutation{UUID: task.UUID, Diff: TaskDiff{Tags: diff}})
	}
	return mutations
}

func calendarDateBefore(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	if leftYear != rightYear {
		return leftYear < rightYear
	}
	if leftMonth != rightMonth {
		return leftMonth < rightMonth
	}
	return leftDay < rightDay
}

func locationOrLocal(value *time.Location) *time.Location {
	if value == nil {
		return time.Local
	}
	return value
}

func maxInt(value, fallback int) int {
	if value > fallback {
		return value
	}
	return fallback
}
