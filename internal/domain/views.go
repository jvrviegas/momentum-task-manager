package domain

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// TodayGroup identifies the mutually exclusive sections in the Today view.
type TodayGroup string

const (
	GroupOverdue        TodayGroup = "Overdue"
	GroupDueToday       TodayGroup = "Due Today"
	GroupScheduledToday TodayGroup = "Scheduled Today"
)

// TodaySection is one rendered section of the Today view.
type TodaySection struct {
	Group TodayGroup
	Tasks []Task
}

// Views contains the two overlapping, locally-derived task views.
type Views struct {
	Inbox    []Task
	Today    []Task
	Sections []TodaySection
}

// BuildViews derives Inbox and Today from one consistent pending export.
func BuildViews(tasks []Task, now time.Time) Views {
	if now.IsZero() {
		now = time.Now()
	}

	inbox := make([]Task, 0, len(tasks))
	groups := map[TodayGroup][]Task{
		GroupOverdue:        {},
		GroupDueToday:       {},
		GroupScheduledToday: {},
	}
	for _, task := range tasks {
		if !task.IsPending() {
			continue
		}
		inbox = append(inbox, task)
		if group := ClassifyToday(task, now); group != "" {
			groups[group] = append(groups[group], task)
		}
	}

	SortTasks(inbox)
	sections := make([]TodaySection, 0, 3)
	today := make([]Task, 0)
	for _, group := range []TodayGroup{GroupOverdue, GroupDueToday, GroupScheduledToday} {
		sectionTasks := groups[group]
		SortTasks(sectionTasks)
		if len(sectionTasks) == 0 {
			continue
		}
		sectionTasks = slices.Clone(sectionTasks)
		sections = append(sections, TodaySection{Group: group, Tasks: sectionTasks})
		today = append(today, sectionTasks...)
	}
	return Views{Inbox: inbox, Today: today, Sections: sections}
}

// DeriveViews is an explicit alias for callers that prefer the domain wording.
func DeriveViews(tasks []Task, now time.Time) Views {
	return BuildViews(tasks, now)
}

// ClassifyToday applies the approved due-before-scheduled precedence using the
// calendar date in now's location.
func ClassifyToday(task Task, now time.Time) TodayGroup {
	if !task.IsPending() {
		return ""
	}
	if now.IsZero() {
		now = time.Now()
	}
	loc := now.Location()
	today := now.In(loc)
	if due, ok := DateIn(task.Due, loc); ok {
		dueDate := dateOnly(due)
		todayDate := dateOnly(today)
		switch {
		case dueDate.Before(todayDate):
			return GroupOverdue
		case dueDate.Equal(todayDate):
			return GroupDueToday
		}
	}
	if scheduled, ok := DateIn(task.Scheduled, loc); ok {
		if dateOnly(scheduled).Equal(dateOnly(today)) {
			return GroupScheduledToday
		}
	}
	return ""
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

// SortTasks sorts by urgency descending and uses stable task identity fields to
// make equal-urgency exports deterministic.
func SortTasks(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Urgency != tasks[j].Urgency {
			return tasks[i].Urgency > tasks[j].Urgency
		}
		if tasks[i].UUID != tasks[j].UUID {
			return strings.ToLower(tasks[i].UUID) < strings.ToLower(tasks[j].UUID)
		}
		if tasks[i].Description != tasks[j].Description {
			return tasks[i].Description < tasks[j].Description
		}
		return tasks[i].ID < tasks[j].ID
	})
}

// RestoreSelection returns the new index for a UUID, or a safe nearest index
// when the selected task disappeared during refresh.
func RestoreSelection(tasks []Task, uuid string, previousIndex ...int) int {
	if len(tasks) == 0 {
		return -1
	}
	if uuid != "" {
		for index, task := range tasks {
			if task.UUID == uuid {
				return index
			}
		}
	}
	index := 0
	if len(previousIndex) > 0 {
		index = previousIndex[0]
	}
	if index >= len(tasks) {
		index = len(tasks) - 1
	}
	if index < 0 {
		index = 0
	}
	return index
}

// SelectedUUID returns the identity at index, if one exists.
func SelectedUUID(tasks []Task, index int) string {
	if index < 0 || index >= len(tasks) {
		return ""
	}
	return tasks[index].UUID
}
