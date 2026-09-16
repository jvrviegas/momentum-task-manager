package domain

import (
	"reflect"
	"testing"
)

func TestProjectCatalogPlanMapsOnlyEligiblePendingTasks(t *testing.T) {
	catalog := ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
	}
	plan, err := PlanUpdateProject(catalog, "work", Project{Name: "Delivery", Value: "delivery"}, true)
	if err != nil {
		t.Fatal(err)
	}

	tasks := []Task{
		{UUID: "exact", Status: "pending", Project: "work", Description: "exact"},
		{UUID: "child", Status: "pending", Project: "work.client", Description: "child"},
		{UUID: "unconfigured-child", Status: "pending", Project: "work.unlisted.deep", Description: "unlisted"},
		{UUID: "lookalike", Status: "pending", Project: "workshop", Description: "lookalike"},
		{UUID: "completed", Status: "completed", Project: "work", Description: "completed"},
		{UUID: "deleted", Status: "deleted", Project: "work", Description: "deleted"},
		{UUID: "waiting", Status: "waiting", Project: "work", Description: "waiting"},
		{UUID: "recurring", Status: "pending", Project: "work", Recurrence: "weekly", Description: "recurring"},
		{Status: "pending", Project: "work", Description: "missing uuid"},
		{UUID: "unrelated", Status: "pending", Project: "personal", Description: "unrelated"},
	}

	got := plan.PendingTaskMappings(tasks)
	want := []ProjectTaskMapping{
		{UUID: "exact", OldValue: "work", NewValue: "delivery"},
		{UUID: "child", OldValue: "work.client", NewValue: "delivery.client"},
		{UUID: "unconfigured-child", OldValue: "work.unlisted.deep", NewValue: "delivery.unlisted.deep"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mappings=%#v want=%#v", got, want)
	}
	if tasks[0].Project != "work" || tasks[1].Description != "child" {
		t.Fatal("task snapshots were mutated")
	}
}

func TestMapPendingProjectTasksHonorsExactBoundaryAndSubprojectOption(t *testing.T) {
	tasks := []Task{
		{UUID: "exact", Status: "pending", Project: "work"},
		{UUID: "child", Status: "pending", Project: "work.client"},
		{UUID: "lookalike", Status: "pending", Project: "workshop"},
	}

	exactOnly := MapPendingProjectTasks(tasks, "work", "delivery", false)
	if want := []ProjectTaskMapping{{UUID: "exact", OldValue: "work", NewValue: "delivery"}}; !reflect.DeepEqual(exactOnly, want) {
		t.Fatalf("exact-only mappings=%#v want=%#v", exactOnly, want)
	}

	withChildren := MapPendingProjectTasks(tasks, "work", "delivery", true)
	want := []ProjectTaskMapping{
		{UUID: "exact", OldValue: "work", NewValue: "delivery"},
		{UUID: "child", OldValue: "work.client", NewValue: "delivery.client"},
	}
	if !reflect.DeepEqual(withChildren, want) {
		t.Fatalf("subproject mappings=%#v want=%#v", withChildren, want)
	}
}

func TestMapPendingProjectTasksReturnsNoMatchesWithoutBroadeningScope(t *testing.T) {
	tasks := []Task{{UUID: "other", Status: "pending", Project: "personal"}}
	got := MapPendingProjectTasks(tasks, "work", "delivery", true)
	if got == nil || len(got) != 0 {
		t.Fatalf("mappings=%#v want an empty result", got)
	}
}

func TestProjectCatalogPlanDoesNotOfferTaskMappingsForLabelOnlyOrNoOp(t *testing.T) {
	catalog := ProjectCatalog{{Name: "Work", Value: "work"}}
	for _, project := range []Project{
		{Name: "Work", Value: "work"},
		{Name: "Delivery", Value: "work"},
	} {
		plan, err := PlanUpdateProject(catalog, "work", project, true)
		if err != nil {
			t.Fatal(err)
		}
		if plan.ValueChanged() || len(plan.PendingTaskMappings([]Task{{UUID: "task", Status: "pending", Project: "work"}})) != 0 {
			t.Fatalf("plan=%#v unexpectedly maps tasks", plan)
		}
	}
}
