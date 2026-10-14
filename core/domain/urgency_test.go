package domain

import (
	"testing"
	"time"
)

// The expected values are what `task export` printed in Taskwarrior 3.5.0 at
// 2026-10-14 17:37:29 UTC for these tasks, imported with the same offsets
// from that second. Each task's UUID is its description, and entry is that
// second unless set.
func TestSetUrgencyMatchesTaskwarrior(t *testing.T) {
	base := time.Unix(1791999449, 0).UTC()
	day, hour := 24*time.Hour, time.Hour
	at := func(offset time.Duration) *time.Time {
		value := base.Add(offset)
		return &value
	}
	notes := func(count int) []Annotation { return make([]Annotation, count) }
	fixtures := []struct {
		task Task
		want float64
	}{
		{Task{Description: "plain"}, 0},
		{Task{Description: "project", Project: "work.client"}, 1},
		{Task{Description: "priority H", Priority: "H"}, 6},
		{Task{Description: "priority M", Priority: "M"}, 3.9},
		{Task{Description: "priority L", Priority: "L"}, 1.8},
		{Task{Description: "one tag", Tags: []string{"a"}}, 0.8},
		{Task{Description: "two tags", Tags: []string{"a", "b"}}, 0.9},
		{Task{Description: "three tags", Tags: []string{"a", "b", "c"}}, 1},
		{Task{Description: "next tag", Tags: []string{"next"}}, 15.8},
		{Task{Description: "one annotation", Annotations: notes(1)}, 0.8},
		{Task{Description: "two annotations", Annotations: notes(2)}, 0.9},
		{Task{Description: "three annotations", Annotations: notes(3)}, 1},
		{Task{Description: "four annotations", Annotations: notes(4)}, 1},
		{Task{Description: "due in 20 days", Due: at(20 * day)}, 2.4},
		{Task{Description: "due in 14 days", Due: at(14 * day)}, 2.4},
		{Task{Description: "due in 3 days 5 hours", Due: at(3*day + 5*hour)}, 7.33333},
		{Task{Description: "due now", Due: at(0)}, 8.8},
		{Task{Description: "overdue 2 days 7 hours", Due: at(-(2*day + 7*hour))}, 9.84762},
		{Task{Description: "overdue 7 days", Due: at(-7 * day)}, 12},
		{Task{Description: "overdue 30 days", Due: at(-30 * day)}, 12},
		{Task{Description: "scheduled an hour ago", Scheduled: at(-hour)}, 5},
		{Task{Description: "scheduled now", Scheduled: at(0)}, 0},
		{Task{Description: "scheduled in an hour", Scheduled: at(hour)}, 0},
		{Task{Description: "waiting", Wait: at(2 * day)}, 5},
		{Task{Description: "blocked by waiting", Dependencies: []string{"waiting"}}, -5},
		{Task{Description: "wait passed", Wait: at(-day)}, 0},
		{Task{Description: "active", Start: at(-hour)}, 4},
		{Task{Description: "blocked", Dependencies: []string{"middle"}}, -5},
		{Task{Description: "middle", Dependencies: []string{"blocker"}}, 3},
		{Task{Description: "blocker"}, 8},
		{Task{Description: "blocked by done", Dependencies: []string{"done"}}, 0},
		{Task{Description: "done", Status: "completed", End: at(-hour)}, 0},
		{Task{Description: "blocked by deleted", Dependencies: []string{"deleted"}}, 0},
		{Task{Description: "deleted", Status: "deleted", End: at(-hour)}, 0},
		{Task{Description: "half a day old", Entry: at(-12 * hour)}, 0},
		{Task{Description: "10 days old", Entry: at(-(10*day + hour))}, 0.0547945},
		{Task{Description: "200 days old", Entry: at(-200 * day)}, 1.09589},
		{Task{Description: "365 days old", Entry: at(-365 * day)}, 2},
		{Task{Description: "400 days old", Entry: at(-400 * day)}, 2},
		{Task{Description: "entry in 3 days", Entry: at(3 * day)}, -0.0164384},
		{Task{
			Description: "everything", Project: "home", Priority: "H", Tags: []string{"next", "x"}, Annotations: notes(2),
			Due: at(-day), Scheduled: at(-2 * hour), Start: at(-hour), Entry: at(-30 * day),
		}, 50.2215},
		{Task{Description: "needs everything", Dependencies: []string{"everything"}}, -5},
		// Summing in float64, or fusing a product into the sum, changes the
		// sixth digit of these.
		{Task{Description: "edge 1", Project: "p", Tags: []string{"a"}, Due: at(1209504 * time.Second), Entry: at(1209504 * time.Second)}, 4.12928},
		{Task{Description: "edge 2", Project: "p", Tags: []string{"a"}, Due: at(1209487 * time.Second), Entry: at(1209487 * time.Second)}, 4.12936},
		{Task{Description: "edge 3", Project: "p", Tags: []string{"a"}, Due: at(1209470 * time.Second), Entry: at(1209470 * time.Second)}, 4.12946},
		{Task{Description: "edge 4", Project: "p", Tags: []string{"a"}, Due: at(1209402 * time.Second), Entry: at(1209402 * time.Second)}, 4.12982},
		{Task{Description: "edge 5", Project: "p", Tags: []string{"a"}, Due: at(1209298 * time.Second), Entry: at(1209298 * time.Second)}, 4.13036},
		{Task{Description: "edge 6", Project: "p", Tags: []string{"a"}, Due: at(1209281 * time.Second), Entry: at(1209281 * time.Second)}, 4.13046},
	}
	tasks := make([]Task, len(fixtures))
	for index, fixture := range fixtures {
		task := fixture.task
		task.UUID = task.Description
		if task.Status == "" {
			task.Status = "pending"
		}
		if task.Entry == nil {
			task.Entry = at(0)
		}
		tasks[index] = task
	}

	// Taskwarrior counts whole seconds, so a fraction of one changes nothing;
	// "scheduled now" would otherwise be in the past.
	SetUrgency(tasks, base.Add(999*time.Millisecond))

	for index, task := range tasks {
		if want := fixtures[index].want; task.Urgency != want {
			t.Errorf("%s: urgency %v, Taskwarrior %v", task.Description, task.Urgency, want)
		}
	}
}

func TestSetUrgencyIgnoresDependenciesOutsideTasks(t *testing.T) {
	now := time.Date(2026, 10, 14, 17, 0, 0, 0, time.UTC)
	tasks := []Task{{UUID: "a", Status: "pending", Entry: &now, Dependencies: []string{"missing"}}}
	SetUrgency(tasks, now)
	if tasks[0].Urgency != 0 {
		t.Fatalf("urgency %v, want 0: a dependency outside tasks neither blocks nor counts", tasks[0].Urgency)
	}
}
