package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

func TestEditShortcutFieldMap(t *testing.T) {
	cases := map[string]ui.EditField{"e": ui.FieldDescription, "p": ui.FieldProject, "!": ui.FieldPriority, "D": ui.FieldDue, "S": ui.FieldScheduled, "t": ui.FieldTags}
	for key, want := range cases {
		got, ok := EditShortcutField(key)
		if !ok || got != want {
			t.Errorf("key=%q got=%d ok=%v want=%d", key, got, ok, want)
		}
	}
	if _, ok := EditShortcutField("x"); ok {
		t.Fatal("unknown shortcut should not map")
	}
}

func TestOpenEditShortcutTargetsSelectedTask(t *testing.T) {
	model := actionModel(&fakeClient{})
	for _, key := range []string{"e", "p", "!", "D", "S", "t"} {
		model.Overlay = OverlayNone
		model.MutationRunning = false
		model.OpenEditShortcut(key)
		if model.Overlay != OverlayEdit || !model.Editor.Open {
			t.Errorf("key=%q model=%#v", key, model)
		}
		model.Editor.Close()
	}
}

func TestSubmitEditWithEmptyDiffDoesNotCallClient(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	if cmd := model.SubmitEdit(ui.EditSubmitMsg{Diff: domain.TaskDiff{}}); cmd != nil || model.MutationRunning || len(client.mutations) != 0 || model.Overlay != OverlayNone {
		t.Fatalf("model=%#v mutations=%#v", model, client.mutations)
	}
}

func TestSubmitEditInvokesOneModifyWithSelectedUUID(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	diff := domain.TaskDiff{Project: domain.FieldChange{Kind: domain.Set, Value: "new"}}
	cmd := model.SubmitEdit(ui.EditSubmitMsg{Diff: diff})
	if cmd == nil || model.PendingMutation == nil || model.PendingMutation.Kind != MutationModify || model.PendingMutation.UUID != "one" {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
	result := cmd().(MutationMsg)
	if result.Err != nil || len(client.mutations) != 1 || client.mutations[0].Diff.Project.Value != "new" {
		t.Fatalf("result=%#v mutations=%#v", result, client.mutations)
	}
}

func TestRejectedEditRetainsModalInputAndShowsError(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	model.Editor.Inputs[ui.FieldDescription].SetValue("user input retained")
	model.PendingMutation = &MutationRequest{Kind: MutationModify, UUID: "one"}
	model.MutationRunning = true
	err := errors.New("hook rejected edit")
	model.Update(MutationMsg{Kind: MutationModify, UUID: "one", Err: err})
	if !model.Editor.Open || model.Editor.Input(ui.FieldDescription).Value() != "user input retained" || model.Editor.Err != err || !strings.Contains(model.Status, "hook rejected") {
		t.Fatalf("model=%#v", model)
	}
}

func TestSuccessfulEditClosesModalAfterRefreshStarts(t *testing.T) {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{}}})
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	model.PendingMutation = &MutationRequest{Kind: MutationModify, UUID: "one"}
	model.MutationRunning = true
	_, cmd := model.Update(MutationMsg{Kind: MutationModify, UUID: "one"})
	if model.Editor.Open || model.Overlay != OverlayNone || cmd == nil || model.Mode != ModeRefreshing {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestEditErrorTextIsConcise(t *testing.T) {
	if EditErrorText(nil) != "" {
		t.Fatal("nil error should be empty")
	}
	if got := EditErrorText(errors.New(strings.Repeat("x", 200))); len(got) > 160 || !strings.HasSuffix(got, "...") {
		t.Fatalf("text length=%d value=%q", len(got), got)
	}
}

func TestUnsupportedFieldsAreAbsentFromAppEditRequest(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	message := ui.EditSubmitMsg{Diff: domain.TaskDiff{Description: domain.FieldChange{Kind: domain.Set, Value: "new"}}}
	model.SubmitEdit(message)
	if model.PendingMutation.Diff.Project.Kind != domain.Unchanged || model.PendingMutation.Diff.Tags.Changed {
		t.Fatalf("unexpected unsupported changes=%#v", model.PendingMutation.Diff)
	}
}

func TestEditSubmissionMessageRoutesThroughUpdate(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Overlay = OverlayEdit
	model.Editor.Open = true
	_, cmd := model.Update(ui.EditSubmitMsg{Diff: domain.TaskDiff{Due: domain.FieldChange{Kind: domain.Clear}}})
	if cmd == nil || model.PendingMutation.Kind != MutationModify {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestEditShortcutDoesNotMutateWhenNoTaskSelected(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Views = domain.Views{}
	model.Selections[ViewInbox] = -1
	if cmd := model.OpenEditShortcut("p"); cmd != nil || model.Overlay != OverlayNone {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}
