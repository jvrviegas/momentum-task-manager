package app

import (
	"errors"
	"testing"
	"time"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestRefreshIsBlockedDuringMutationAndOverlay(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Mode = ModeMutating
	model.MutationRunning = true
	if _, cmd := model.Update(RefreshRequestedMsg{Reason: "timer"}); cmd != nil {
		t.Fatal("refresh should be blocked during mutation")
	}
	model.Mode = ModeReady
	model.Overlay = OverlayEdit
	if _, cmd := model.Update(RefreshRequestedMsg{Reason: "timer"}); cmd != nil {
		t.Fatal("refresh should be blocked by input overlay")
	}
}

func TestSelectionUUIDIsRestoredPerView(t *testing.T) {
	model := testModel(&fakeClient{})
	due := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	model.Tasks = []domain.Task{{UUID: "inbox", Status: "pending"}, {UUID: "today", Status: "pending", Due: &due}}
	model.Views = domain.Views{
		Inbox: []domain.Task{model.Tasks[0]},
		Today: []domain.Task{model.Tasks[1]},
	}
	model.Selected[ViewInbox] = "inbox"
	model.Selected[ViewToday] = "today"
	model.ActiveView = ViewToday
	model.applyTasks(TasksMsg{Tasks: model.Tasks, Reason: "refresh"})
	if model.Selected[ViewInbox] != "inbox" || model.Selected[ViewToday] != "today" {
		t.Fatalf("selection map=%#v", model.Selected)
	}
}

func TestNilClientProducesTypedCommandErrorMessage(t *testing.T) {
	model := testModel(nil)
	message, ok := LoadTasksCommand(nil, nil, "initial")().(TasksMsg)
	if !ok || !errors.Is(message.Err, errNilClient) || model.Client != nil {
		t.Fatalf("message=%#v model client=%v", message, model.Client)
	}
}
