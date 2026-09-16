package domain

import (
	"testing"
	"time"
)

func TestBuildViewsDeduplicatesDuplicateUUIDsInToday(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	task := taskAt("same", "pending", 3, "2026-09-08 08:00", "2026-09-08 09:00", loc)
	views := BuildViews([]Task{task, task}, now)
	if len(views.Inbox) != 2 {
		t.Fatalf("Inbox should retain every exported pending task, got %d", len(views.Inbox))
	}
	if len(views.Today) != 1 || len(views.Sections) != 1 {
		t.Fatalf("Today should be unique, got %#v", views)
	}
}
