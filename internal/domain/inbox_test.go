package domain

import (
	"testing"
	"time"
)

func TestInboxProjectAndCreationOrderWithoutMutatingInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	old := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	tasks := []Task{
		{UUID: "none", Status: "pending", Entry: &old, Due: &future},
		{UUID: "beta", Status: "pending", Project: "beta", Entry: &old, Due: &future},
		{UUID: "missing", Status: "pending", Project: "Alpha", Due: &future},
		{UUID: "new", Status: "pending", Project: "Alpha", Entry: &now, Due: &future, Urgency: 100},
		{UUID: "old-b", Status: "pending", Project: "Alpha", Entry: &old, Due: &future},
		{UUID: "old-a", Status: "pending", Project: "Alpha", Entry: &old, Due: &future},
	}
	views := BuildViews(tasks, now)
	want := []string{"old-a", "old-b", "new", "missing", "beta", "none"}
	for i, uuid := range want {
		if views.Inbox[i].UUID != uuid {
			t.Fatalf("inbox[%d]=%s, want %s", i, views.Inbox[i].UUID, uuid)
		}
	}
	if len(views.Today) != 0 {
		t.Fatalf("future tasks should remain outside Today: %#v", views.Today)
	}
	if tasks[0].UUID != "none" {
		t.Fatal("BuildViews mutated input order")
	}
}
