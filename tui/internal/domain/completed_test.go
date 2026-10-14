package domain

import (
	"testing"
	"time"
)

func TestBuildCompletedViewGroupsRecentTasksNewestFirst(t *testing.T) {
	loc := time.FixedZone("local", 2*60*60)
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, loc)
	at := func(year int, month time.Month, day, hour int) *time.Time {
		value := time.Date(year, month, day, hour, 0, 0, 0, loc)
		return &value
	}
	tasks := []Task{
		{UUID: "today-old", Status: "completed", End: at(2026, 9, 8, 8)},
		{UUID: "today-new", Status: "completed", End: at(2026, 9, 8, 10)},
		{UUID: "yesterday", Status: "completed", End: at(2026, 9, 7, 20)},
		{UUID: "earlier", Status: "completed", End: at(2026, 8, 10, 9)},
		{UUID: "too-old", Status: "completed", End: at(2026, 8, 9, 23)},
		{UUID: "pending", Status: "pending", End: at(2026, 9, 8, 11)},
		{UUID: "missing-end", Status: "completed"},
	}

	completed, sections := BuildCompletedView(tasks, now, 30)
	if len(completed) != 4 || len(sections) != 3 {
		t.Fatalf("completed=%#v sections=%#v", completed, sections)
	}
	want := []string{"today-new", "today-old", "yesterday", "earlier"}
	for index, uuid := range want {
		if completed[index].UUID != uuid {
			t.Fatalf("completed[%d]=%q want %q", index, completed[index].UUID, uuid)
		}
	}
	if sections[0].Group != GroupCompletedToday || sections[1].Group != GroupCompletedYesterday || sections[2].Group != GroupCompletedEarlier {
		t.Fatalf("groups=%#v", sections)
	}
}
