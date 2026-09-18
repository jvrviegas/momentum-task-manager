package domain

import (
	"testing"
	"time"
)

func taskAt(uuid, status string, urgency float64, due, scheduled string, loc *time.Location) Task {
	task := Task{UUID: uuid, Status: status, Urgency: urgency, Description: uuid}
	if due != "" {
		value, err := time.ParseInLocation("2006-01-02 15:04", due, loc)
		if err != nil {
			panic(err)
		}
		task.Due = &value
	}
	if scheduled != "" {
		value, err := time.ParseInLocation("2006-01-02 15:04", scheduled, loc)
		if err != nil {
			panic(err)
		}
		task.Scheduled = &value
	}
	return task
}

func TestClassifyTodayPrecedence(t *testing.T) {
	loc := time.FixedZone("test", -4*60*60)
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	cases := []struct {
		name      string
		task      Task
		wantGroup TodayGroup
	}{
		{name: "overdue due", task: taskAt("overdue", "pending", 1, "2026-09-07 23:59", "", loc), wantGroup: GroupOverdue},
		{name: "due at midnight", task: taskAt("due", "pending", 1, "2026-09-08 00:00", "", loc), wantGroup: GroupDueToday},
		{name: "due later", task: taskAt("due-later", "pending", 1, "2026-09-08 23:59", "", loc), wantGroup: GroupDueToday},
		{name: "scheduled only", task: taskAt("scheduled", "pending", 1, "", "2026-09-08 08:00", loc), wantGroup: GroupScheduledToday},
		{name: "due wins over scheduled", task: taskAt("both", "pending", 1, "2026-09-07 08:00", "2026-09-08 08:00", loc), wantGroup: GroupOverdue},
		{name: "due today wins over scheduled", task: taskAt("both-today", "pending", 1, "2026-09-08 08:00", "2026-09-08 08:00", loc), wantGroup: GroupDueToday},
		{name: "future due scheduled today", task: taskAt("future-due", "pending", 1, "2026-09-09 08:00", "2026-09-08 08:00", loc), wantGroup: GroupScheduledToday},
		{name: "scheduled yesterday", task: taskAt("old-schedule", "pending", 1, "", "2026-09-07 08:00", loc), wantGroup: ""},
		{name: "future dates", task: taskAt("future", "pending", 1, "2026-09-09 08:00", "2026-09-09 08:00", loc), wantGroup: ""},
		{name: "completed ignored", task: taskAt("done", "completed", 1, "2026-09-07 08:00", "2026-09-08 08:00", loc), wantGroup: ""},
		{name: "empty dates", task: taskAt("empty", "pending", 1, "", "", loc), wantGroup: ""},
		{name: "date uses local calendar", task: taskAt("boundary", "pending", 1, "2026-09-08 04:30", "", time.UTC), wantGroup: GroupDueToday},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyToday(tc.task, now); got != tc.wantGroup {
				t.Fatalf("got %q, want %q", got, tc.wantGroup)
			}
		})
	}
}

func TestBuildViewsFiltersPendingAndGroupsWithoutDuplicates(t *testing.T) {
	loc := time.FixedZone("test", 2*60*60)
	now := time.Date(2026, 9, 8, 23, 59, 0, 0, loc)
	tasks := []Task{
		taskAt("scheduled", "pending", 2, "", "2026-09-08 08:00", loc),
		taskAt("overdue", "pending", 10, "2026-09-07 08:00", "2026-09-08 08:00", loc),
		taskAt("done", "completed", 100, "2026-09-07 08:00", "", loc),
		taskAt("due", "pending", 5, "2026-09-08 08:00", "", loc),
		taskAt("inbox", "pending", 1, "2026-09-09 08:00", "", loc),
	}
	views := BuildViews(tasks, now)
	if len(views.Inbox) != 1 || len(views.Today) != 3 || len(views.Sections) != 3 {
		t.Fatalf("unexpected view sizes: inbox=%d today=%d sections=%d", len(views.Inbox), len(views.Today), len(views.Sections))
	}
	if views.Sections[0].Group != GroupOverdue || views.Sections[1].Group != GroupDueToday || views.Sections[2].Group != GroupScheduledToday {
		t.Fatalf("unexpected section order: %#v", views.Sections)
	}
	if views.Inbox[0].UUID != "inbox" {
		t.Fatalf("Today-classified tasks leaked into Inbox: %#v", views.Inbox)
	}
	if views.Today[0].UUID != "overdue" || views.Today[1].UUID != "due" || views.Today[2].UUID != "scheduled" {
		t.Fatalf("unexpected flattened order: %#v", views.Today)
	}
}

func TestBuildViewsSortsInboxWithDeterministicTies(t *testing.T) {
	tasks := []Task{
		{UUID: "b", Description: "B", Status: "pending", Urgency: 3},
		{UUID: "z", Description: "Z", Status: "pending", Urgency: 10},
		{UUID: "a", Description: "A", Status: "pending", Urgency: 10},
		{UUID: "c", Description: "C", Status: "completed", Urgency: 99},
	}
	views := BuildViews(tasks, time.Now())
	got := []string{views.Inbox[0].UUID, views.Inbox[1].UUID, views.Inbox[2].UUID}
	want := []string{"a", "b", "z"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v, want=%v", got, want)
		}
	}
}

func TestBuildViewsDoesNotDuplicateAGroupMatch(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, loc)
	task := taskAt("one", "pending", 1, "2026-09-08 08:00", "2026-09-08 09:00", loc)
	views := BuildViews([]Task{task}, now)
	if len(views.Today) != 1 || len(views.Sections) != 1 || views.Sections[0].Group != GroupDueToday {
		t.Fatalf("task should appear once in due section: %#v", views)
	}
}

func TestRestoreSelectionByUUIDAndNearestFallback(t *testing.T) {
	tasks := []Task{{UUID: "a"}, {UUID: "b"}, {UUID: "c"}}
	if got := RestoreSelection(tasks, "c", 0); got != 2 {
		t.Fatalf("uuid selection=%d", got)
	}
	if got := RestoreSelection(tasks[:2], "c", 2); got != 1 {
		t.Fatalf("fallback selection=%d", got)
	}
	if got := RestoreSelection(nil, "a", 0); got != -1 {
		t.Fatalf("empty selection=%d", got)
	}
	if got := SelectedUUID(tasks, 1); got != "b" {
		t.Fatalf("selected uuid=%q", got)
	}
}
