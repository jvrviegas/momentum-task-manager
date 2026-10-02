package app

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

func TestOpenPlannerShowsObligationsAndCandidatesWithoutMutating(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	due := now.Add(time.Hour)
	client := &fakeClient{}
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: func() time.Time { return now }, Context: context.Background(), Width: 100, Height: 30})
	model.Tasks = []domain.Task{
		{UUID: "due", Description: "deadline", Status: "pending", Due: &due, Estimate: &domain.Estimate{Minutes: 60}},
		{UUID: "candidate", Description: "candidate", Status: "pending", Estimate: &domain.Estimate{Minutes: 30}},
	}
	model.Views = domain.BuildViews(model.Tasks, now)
	if cmd := model.OpenPlanner(); cmd != nil {
		// Calendar is disabled by default.
		t.Fatal("unexpected calendar command")
	}
	if model.Overlay != OverlayPlanner || len(model.Planner.Items) != 2 || len(client.mutations) != 0 {
		t.Fatalf("model=%#v mutations=%#v", model.Planner, client.mutations)
	}
}

func TestPlannerConfirmUsesOnlyPlanTagMutations(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{exports: [][]domain.Task{{}}}
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: func() time.Time { return now }, Context: context.Background()})
	model.Tasks = []domain.Task{{UUID: "candidate", Description: "candidate", Status: "pending"}}
	model.Views = domain.BuildViews(model.Tasks, now)
	model.OpenPlanner()
	model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}))
	cmd := model.Planner.Update(tea.KeyPressMsg(tea.Key{Text: "enter", Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("planner did not return submit command")
	}
	_, updateCmd := model.Update(cmd())
	if updateCmd == nil || !model.MutationRunning {
		t.Fatalf("model=%#v cmd=%v", model, updateCmd)
	}
	message, ok := updateCmd().(MutationMsg)
	if !ok || message.Err != nil || len(client.mutations) != 1 {
		t.Fatalf("message=%#v mutations=%#v", message, client.mutations)
	}
	if diff := client.mutations[0].Diff; !diff.Tags.Changed || diff.Due.Kind != domain.Unchanged {
		t.Fatalf("diff=%#v", diff)
	}
}

func TestStopRecurrenceTargetsParentAfterConfirmation(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{}
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), Now: func() time.Time { return now }})
	model.Tasks = []domain.Task{{UUID: "instance", Parent: "template", Description: "daily", Status: "pending", Recurrence: "daily"}}
	model.Views = domain.BuildViews(model.Tasks, now)
	model.Selected[ViewInbox] = "instance"
	model.Selections[ViewInbox] = 0
	if cmd := model.StopRecurrenceSelected(); cmd != nil || model.Overlay != OverlayConfirm {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
	cmd := model.ConfirmDelete(ui.ConfirmYes)
	if cmd == nil {
		t.Fatal("confirmation did not start mutation")
	}
	message := cmd().(MutationMsg)
	if message.Err != nil || len(client.mutations) != 1 || client.mutations[0].UUID != "template" || client.mutations[0].Diff.Recurrence.Kind != domain.Clear {
		t.Fatalf("message=%#v mutations=%#v", message, client.mutations)
	}
}
