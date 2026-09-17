package domain

import (
	"testing"
	"time"
)

func TestInboxProjectAndCreationOrderLeavesTodayUnchanged(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	old := now.Add(-24 * time.Hour)
	tasks := []Task{
		{UUID: "none", Status: "pending", Entry: &old, Due: &now},
		{UUID: "beta", Status: "pending", Project: "beta", Entry: &old, Due: &now},
		{UUID: "missing", Status: "pending", Project: "Alpha", Due: &now},
		{UUID: "new", Status: "pending", Project: "Alpha", Entry: &now, Due: &now, Urgency: 100},
		{UUID: "old-b", Status: "pending", Project: "Alpha", Entry: &old, Due: &now},
		{UUID: "old-a", Status: "pending", Project: "Alpha", Entry: &old, Due: &now},
	}
	views := BuildViews(tasks, now)
	want := []string{"old-a", "old-b", "new", "missing", "beta", "none"}
	for i, uuid := range want {
		if views.Inbox[i].UUID != uuid {
			t.Fatalf("inbox[%d]=%s, want %s", i, views.Inbox[i].UUID, uuid)
		}
	}
	if views.Today[0].UUID != "new" {
		t.Fatal("Today must still sort by urgency")
	}
	if tasks[0].UUID != "none" {
		t.Fatal("BuildViews mutated input order")
	}
}
