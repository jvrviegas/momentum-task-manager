package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

type completedCapableClient struct {
	*fakeClient
	completed    []domain.Task
	completedErr error
	after        time.Time
}

func (c *completedCapableClient) ExportCompleted(_ context.Context, after time.Time) ([]domain.Task, error) {
	c.after = after
	return c.completed, c.completedErr
}

func TestCompletedLoadUsesThirtyCalendarDayWindowAndExplicitView(t *testing.T) {
	loc := time.FixedZone("local", 2*60*60)
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, loc)
	end := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	client := &completedCapableClient{
		fakeClient: &fakeClient{exports: [][]domain.Task{{testTask("pending", 1)}}},
		completed:  []domain.Task{{UUID: "done", Description: "Done", Status: "completed", Project: "work", End: &end}},
	}
	model := NewModel(ModelOptions{Client: client, InitialView: ViewCompleted, Now: func() time.Time { return now }, Width: 79, Height: 24})
	message := model.Init()().(TasksMsg)
	model.Update(message)

	wantAfter := time.Date(2026, 8, 9, 0, 0, 0, 0, loc)
	if !client.after.Equal(wantAfter) {
		t.Fatalf("after=%s want=%s", client.after, wantAfter)
	}
	if model.ActiveView != ViewCompleted || len(model.Views.Completed) != 1 || model.Selected[ViewCompleted] != "done" {
		t.Fatalf("active=%s completed=%#v selected=%#v", model.ActiveView, model.Views.Completed, model.Selected)
	}
	content := model.View().Content
	if !strings.Contains(content, "Today · 1") || !strings.Contains(content, "#work") || !strings.Contains(content, "Completed Today 10:00") {
		t.Fatalf("completed view=%q", content)
	}
}

func TestCompletedFailureRetainsPreviousDataAndPendingViews(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.Local)
	end := now.Add(-time.Hour)
	model := NewModel(ModelOptions{Now: func() time.Time { return now }})
	model.applyTasks(TasksMsg{
		Tasks:           []domain.Task{testTask("pending", 1)},
		Completed:       []domain.Task{{UUID: "done", Status: "completed", End: &end}},
		CompletedLoaded: true,
		Reason:          "initial",
	})
	failure := errors.New("archive unavailable")
	model.applyTasks(TasksMsg{Tasks: []domain.Task{testTask("new-pending", 1)}, CompletedLoaded: true, CompletedErr: failure, Reason: "refresh"})

	if len(model.Views.Inbox) != 1 || model.Views.Inbox[0].UUID != "new-pending" {
		t.Fatalf("pending view not refreshed: %#v", model.Views.Inbox)
	}
	if len(model.Views.Completed) != 1 || model.Views.Completed[0].UUID != "done" {
		t.Fatalf("completed data not retained: %#v", model.Views.Completed)
	}
	if model.CompletedErr != failure || !strings.Contains(model.Status, "Completed refresh failed") {
		t.Fatalf("err=%v status=%q", model.CompletedErr, model.Status)
	}
}

func TestCompletedTaskActionsAreReadOnlyButUndoRemainsGlobal(t *testing.T) {
	now := time.Now()
	end := now.Add(-time.Hour)
	client := &fakeClient{}
	model := NewModel(ModelOptions{Client: client, Now: func() time.Time { return now }})
	model.applyTasks(TasksMsg{Completed: []domain.Task{{UUID: "done", Description: "Done", Status: "completed", End: &end}}, CompletedLoaded: true, Reason: "initial"})
	model.SwitchView(ViewCompleted)

	if cmd := model.CompleteSelected(); cmd != nil {
		t.Fatal("completion action should be disabled")
	}
	if cmd := model.OpenEditor(ui.FieldDescription); cmd != nil {
		t.Fatal("editing should be disabled")
	}
	model.DeleteSelected()
	if model.Overlay != OverlayNone || model.Status != "Completed tasks are read-only" {
		t.Fatalf("overlay=%s status=%q", model.Overlay, model.Status)
	}

	model.Sync.UndoAvailable = true
	if cmd := model.UndoLast(); cmd == nil {
		t.Fatal("global undo should remain available")
	}
}
