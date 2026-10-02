package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

func TestQuickAddKeyFlowProducesSubmissionThroughRootModel(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Width, model.Height = 80, 24
	model.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Mod: tea.ModCtrl}))
	if model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open {
		t.Fatalf("not open: overlay=%s open=%v", model.Overlay, model.QuickAdd.Open)
	}
	for _, char := range []rune("Final UAT task #work +testing") {
		model.Update(tea.KeyPressMsg(tea.Key{Code: char, Text: string(char)}))
	}
	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	message, ok := cmd().(ui.QuickAddSubmitMsg)
	if !ok || message.Task.Description != "Final UAT task" || message.Task.Project != "work" || len(message.Task.Tags) != 1 {
		t.Fatalf("message=%#v", message)
	}
}
