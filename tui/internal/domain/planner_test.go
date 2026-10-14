package domain

import (
	"testing"
	"time"
)

func TestBuildDailyPlanSeparatesObligationsRolloverAndCandidates(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	due := now.Add(2 * time.Hour)
	rollover := now.AddDate(0, 0, -1)
	before := Task{UUID: "roll", Description: "rollover", Status: "pending", Tags: []string{DailyPlanTagFor(rollover)}, Urgency: 4}
	obligation := Task{UUID: "due", Description: "deadline", Status: "pending", Due: &due, Estimate: &Estimate{Minutes: 60}, Urgency: 5}
	candidate := Task{UUID: "candidate", Description: "candidate", Status: "pending", Estimate: &Estimate{Minutes: 90}, Urgency: 3}
	plan := BuildDailyPlan([]Task{candidate, obligation, before}, now, PlanningConstraints{CapacityMinutes: 240, BufferMinutes: 30})
	if len(plan.Obligations) != 1 || plan.Obligations[0].Task.UUID != "due" {
		t.Fatalf("obligations=%#v", plan.Obligations)
	}
	if len(plan.Rollover) != 1 || !plan.Rollover[0].Selected {
		t.Fatalf("rollover=%#v", plan.Rollover)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Task.UUID != "candidate" {
		t.Fatalf("candidates=%#v", plan.Candidates)
	}
	if plan.Capacity.FixedMinutes != 60 || plan.Capacity.SelectedMinutes != 0 || plan.Capacity.AvailableMinutes != 150 {
		t.Fatalf("capacity=%#v", plan.Capacity)
	}
}

func TestBuildPlanMutationsAreIdempotentAndDoNotTouchDue(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	due := now.AddDate(0, 0, 3)
	task := Task{UUID: "one", Description: "one", Status: "pending", Due: &due, Tags: []string{"keep", DailyPlanTagFor(now.AddDate(0, 0, -1))}}
	mutations := BuildPlanMutations([]Task{task}, map[string]bool{"one": true}, now)
	if len(mutations) != 1 || mutations[0].Diff.Description.Kind != Unchanged || mutations[0].Diff.Due.Kind != Unchanged {
		t.Fatalf("mutations=%#v", mutations)
	}
	if len(mutations[0].Diff.Tags.Add) != 1 || mutations[0].Diff.Tags.Add[0] != DailyPlanTagFor(now) || len(mutations[0].Diff.Tags.Remove) != 1 {
		t.Fatalf("tag diff=%#v", mutations[0].Diff.Tags)
	}
	if got := DailyPlanTagFor(now); got != "momentum-plan-2026-09-21" {
		t.Fatalf("tag=%q", got)
	}
}

func TestPlannedTodayEntersTodayAfterObligations(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	task := Task{UUID: "planned", Description: "planned", Status: "pending", Tags: []string{DailyPlanTagFor(now)}}
	views := BuildViews([]Task{task}, now)
	if len(views.Inbox) != 0 || len(views.Today) != 1 || len(views.Sections) != 1 || views.Sections[0].Group != GroupPlannedToday {
		t.Fatalf("views=%#v", views)
	}
}
