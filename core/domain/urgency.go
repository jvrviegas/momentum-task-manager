package domain

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// Taskwarrior 3's default urgency coefficients, as `task show urgency` lists
// them. They are float32 because Taskwarrior adds in float.
const (
	urgencyProject     float32 = 1.0
	urgencyActive      float32 = 4.0
	urgencyScheduled   float32 = 5.0
	urgencyWaiting     float32 = -3.0
	urgencyBlocked     float32 = -5.0
	urgencyAnnotations float32 = 1.0
	urgencyTags        float32 = 1.0
	urgencyDue         float32 = 12.0
	urgencyBlocking    float32 = 8.0
	urgencyAge         float32 = 2.0
	urgencyNextTag     float32 = 15.0
	urgencyAgeMaxDays          = 365
)

var urgencyPriority = map[string]float32{"H": 6.0, "M": 3.9, "L": 1.8}

// SetUrgency sets each task's Urgency to what Taskwarrior 3 exports at now
// with its default coefficients, so backend mode orders Inbox and Today as
// Taskwarrior mode does (backend plan D4). A task is blocked by a dependency,
// and blocks it, only when both are in tasks and neither is completed or
// deleted. Pass every unfinished task, including waiting ones that a view
// hides: Taskwarrior counts them as blockers.
func SetUrgency(tasks []Task, now time.Time) {
	unfinished := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if !isFinished(task) {
			unfinished[task.UUID] = true
		}
	}
	blocked := map[string]bool{}
	blocking := map[string]bool{}
	for _, task := range tasks {
		if isFinished(task) {
			continue
		}
		for _, dependency := range task.Dependencies {
			if unfinished[dependency] {
				blocked[task.UUID] = true
				blocking[dependency] = true
			}
		}
	}
	for index := range tasks {
		uuid := tasks[index].UUID
		tasks[index].Urgency = urgency(tasks[index], now.Unix(), blocked[uuid], blocking[uuid])
	}
}

func isFinished(task Task) bool {
	return strings.EqualFold(task.Status, "completed") || strings.EqualFold(task.Status, "deleted")
}

// urgency follows Taskwarrior's Task::urgency_c, term by term and in its
// order, in whole seconds as Taskwarrior counts them. It returns the float32
// sum rounded to the six significant digits that `task export` prints, so
// tasks that tie in Taskwarrior mode tie here too.
func urgency(task Task, now int64, blocked, blocking bool) float64 {
	var sum float32
	add := func(term float64, coefficient float32) {
		// The conversion rounds the product, as Taskwarrior does, instead of
		// letting Go fuse it into the addition.
		sum += float32(float32(term) * coefficient)
	}
	if task.Project != "" {
		add(1, urgencyProject)
	}
	if task.Start != nil {
		add(1, urgencyActive)
	}
	if task.Scheduled != nil && task.Scheduled.Unix() < now {
		add(1, urgencyScheduled)
	}
	if task.Wait != nil && task.Wait.Unix() > now {
		add(1, urgencyWaiting)
	}
	if blocked {
		add(1, urgencyBlocked)
	}
	add(countTerm(len(task.Annotations)), urgencyAnnotations)
	add(countTerm(len(task.Tags)), urgencyTags)
	if task.Due != nil {
		add(dueTerm(now-task.Due.Unix()), urgencyDue)
	}
	if blocking {
		add(1, urgencyBlocking)
	}
	add(ageTerm(task.Entry, now), urgencyAge)
	// Then the per-value coefficients, in the order of their names:
	// urgency.uda.priority.* before urgency.user.tag.next.
	sum += urgencyPriority[task.Priority]
	if slices.Contains(task.Tags, "next") {
		sum += urgencyNextTag
	}
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(float64(sum), 'g', 6, 64), 64)
	return rounded
}

// countTerm scores annotations and tags: 0.8 for one, 0.9 for two, and 1 for
// three or more.
func countTerm(count int) float64 {
	switch count {
	case 0:
		return 0
	case 1:
		return float64(float32(0.8))
	case 2:
		return float64(float32(0.9))
	default:
		return 1
	}
}

// dueTerm maps a due date from two weeks ahead to a week overdue onto 0.2–1.
func dueTerm(overdueSeconds int64) float64 {
	days := float64(float32(float64(overdueSeconds) / 86400))
	switch {
	case days >= 7:
		return 1
	case days >= -14:
		return (days+14)*0.8/21 + 0.2
	default:
		return 0.2
	}
}

// ageTerm grows from 0 to 1 over urgencyAgeMaxDays whole days since entry.
// Taskwarrior always sets entry; without one, it scores as old.
func ageTerm(entry *time.Time, now int64) float64 {
	if entry == nil {
		return 1
	}
	days := (now - entry.Unix()) / 86400
	if days > urgencyAgeMaxDays {
		return 1
	}
	return float64(days) / urgencyAgeMaxDays
}
