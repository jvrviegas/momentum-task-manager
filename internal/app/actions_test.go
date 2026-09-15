package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

func actionModel(client *fakeClient) *Model {
	model := testModel(client)
	model.Tasks = []domain.Task{{UUID: "one", Description: "One", Status: "pending"}, {UUID: "two", Description: "Two", Status: "pending"}}
	model.Views = domain.BuildViews(model.Tasks, model.now())
	model.ActiveView = ViewInbox
	model.Selections[ViewInbox] = 0
	model.Selected[ViewInbox] = "one"
	model.Mode = ModeReady
	return model
}

func TestCompleteSelectedDispatchesSelectedUUID(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	cmd := model.CompleteSelected()
	if cmd == nil || !model.MutationRunning {
		t.Fatal("completion did not start")
	}
	message := cmd().(MutationMsg)
	if message.Kind != MutationComplete || message.UUID != "one" || len(client.mutations) != 1 {
		t.Fatalf("message=%#v mutations=%#v", message, client.mutations)
	}
}

func TestToggleStartSelectedChoosesStartOrStop(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.ToggleStartSelected()()
	if client.mutations[0].Kind != MutationStart {
		t.Fatalf("kind=%s", client.mutations[0].Kind)
	}
	model.MutationRunning = false
	started := time.Now()
	model.Tasks[0].Start = &started
	model.Views = domain.BuildViews(model.Tasks, model.now())
	model.ToggleStartSelected()()
	if client.mutations[1].Kind != MutationStop {
		t.Fatalf("kind=%s", client.mutations[1].Kind)
	}
}

func TestDeleteSelectedOnlyOpensConfirmation(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.DeleteSelected()
	if model.Overlay != OverlayConfirm || !model.Confirm.Open || model.DeleteTarget != "one" {
		t.Fatalf("model=%#v", model)
	}
}

func TestDeleteConfirmationStartsMutationOnlyForYes(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.DeleteSelected()
	model.ConfirmDelete(ui.ConfirmNo)
	if model.MutationRunning || model.Overlay != OverlayNone {
		t.Fatal("no should not mutate")
	}
	model.DeleteSelected()
	cmd := model.ConfirmDelete(ui.ConfirmYes)
	if cmd == nil || !model.MutationRunning || model.PendingMutation.Kind != MutationDelete {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestUndoLastRequiresGraceAvailability(t *testing.T) {
	model := actionModel(&fakeClient{})
	if model.UndoLast() != nil {
		t.Fatal("undo should not run outside grace")
	}
	model.Sync.UndoAvailable = true
	cmd := model.UndoLast()
	if cmd == nil || model.PendingMutation.Kind != MutationUndo {
		t.Fatalf("model=%#v", model)
	}
}

func TestOpenQuickAddAndEditorUseOverlays(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.OpenQuickAdd()
	if model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open {
		t.Fatal("quick add not open")
	}
	model.QuickAdd.Close()
	model.Overlay = OverlayNone
	model.OpenEditor(ui.FieldProject)
	if model.Overlay != OverlayEdit || !model.Editor.Open || model.Editor.Focused != ui.FieldProject {
		t.Fatalf("editor=%#v", model.Editor)
	}
}

func TestOpenDetailsAndHelpUseOverlays(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.OpenDetails()
	if model.Overlay != OverlayDetails || !model.Details.Open {
		t.Fatal("details not open")
	}
	model.Details.Close()
	model.Overlay = OverlayNone
	model.OpenHelp()
	if model.Overlay != OverlayHelp || !model.Help.Open {
		t.Fatal("help not open")
	}
}

func TestDetailsEditKeyTransitionsToEditor(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.OpenDetails()
	model.Update(key("e"))
	if model.Overlay != OverlayEdit || !model.Editor.Open {
		t.Fatalf("model=%#v", model)
	}
}

func TestGlobalActionKeysRouteToExpectedActions(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	for input, want := range map[string]MutationKind{" ": MutationComplete, "s": MutationStart} {
		model.MutationRunning = false
		model.Overlay = OverlayNone
		model.Update(key(input))
		if model.PendingMutation == nil || model.PendingMutation.Kind != want {
			t.Errorf("key %q pending=%#v", input, model.PendingMutation)
		}
	}
}

func TestEditorSubmissionUsesSelectedUUID(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	message := ui.EditSubmitMsg{Diff: domain.TaskDiff{Project: domain.FieldChange{Kind: domain.Set, Value: "new"}}}
	cmd := model.handleEditSubmit(message)
	if cmd == nil || model.PendingMutation == nil || model.PendingMutation.UUID != "one" || model.PendingMutation.Kind != MutationModify {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestEmptyEditClosesWithoutClientCall(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	if cmd := model.handleEditSubmit(ui.EditSubmitMsg{Diff: domain.TaskDiff{}}); cmd != nil || model.Overlay != OverlayNone || len(client.mutations) != 0 {
		t.Fatalf("model=%#v mutations=%#v", model, client.mutations)
	}
}

func TestSuccessfulMutationMarksSyncGraceAndRequestsRefresh(t *testing.T) {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{}}})
	model.PendingMutation = &MutationRequest{Kind: MutationComplete, UUID: "one"}
	model.MutationRunning = true
	_, cmd := model.Update(MutationMsg{Kind: MutationComplete, UUID: "one"})
	if cmd == nil || model.Mode != ModeRefreshing || !model.Sync.Unsynced {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestFailedDeleteLeavesTaskAndStatus(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.PendingMutation = &MutationRequest{Kind: MutationDelete, UUID: "one"}
	model.MutationRunning = true
	model.Update(MutationMsg{Kind: MutationDelete, UUID: "one", Err: errActionTest})
	if model.Mode != ModeReady || model.Err != errActionTest || model.Status == "" {
		t.Fatalf("model=%#v", model)
	}
}

func TestTickCommandCanBeDisabled(t *testing.T) {
	if TickCommand(0) != nil || TickCommand(-time.Second) != nil {
		t.Fatal("disabled tick should be nil")
	}
	if TickCommand(time.Hour) == nil {
		t.Fatal("positive tick should return command")
	}
}

func TestOverlayPreventsGlobalMutationKey(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayHelp
	model.Help.OpenHelp()
	model.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: tea.KeySpace}))
	if model.MutationRunning || model.PendingMutation != nil {
		t.Fatal("overlay allowed global mutation")
	}
}

var errActionTest = &actionError{}

type actionError struct{}

func (*actionError) Error() string { return "action failed" }
