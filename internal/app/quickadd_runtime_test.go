package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

type estimateAddClient struct {
	*fakeClient
	err error
}

func (c *estimateAddClient) Add(ctx context.Context, input domain.NewTask) error {
	if input.Estimate != nil && c.err != nil {
		return c.err
	}
	return c.fakeClient.Add(ctx, input)
}

func TestQuickAddEstimateFailureRemainsActionableAtAppSeam(t *testing.T) {
	client := &estimateAddClient{fakeClient: &fakeClient{}, err: &taskwarrior.EstimateUDAError{State: taskwarrior.EstimateUDAMissing}}
	model := testModel(client)
	model.Mode = ModeReady
	model.Overlay = OverlayQuickAdd
	model.QuickAdd.Open = true
	estimate := &domain.Estimate{Minutes: 60}
	_, cmd := model.Update(ui.QuickAddSubmitMsg{Task: domain.NewTask{Description: "write docs", Estimate: estimate}})
	if cmd == nil {
		t.Fatal("estimate submission did not start mutation")
	}
	result, ok := cmd().(MutationMsg)
	if !ok || result.Err == nil {
		t.Fatalf("result=%#v", result)
	}
	model.Update(result)
	if !strings.Contains(model.Status, "uda.estimate.type=duration") || len(client.mutations) != 0 {
		t.Fatalf("status=%q mutations=%#v", model.Status, client.mutations)
	}
}

func TestQuickAddKeyFlowProducesSubmissionThroughRootModel(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Width, model.Height = 80, 24
	model.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Mod: tea.ModCtrl}))
	if model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open {
		t.Fatalf("not open: overlay=%s open=%v", model.Overlay, model.QuickAdd.Open)
	}
	for _, char := range []rune("Final UAT task #work +testing ~1h30m") {
		model.Update(tea.KeyPressMsg(tea.Key{Code: char, Text: string(char)}))
	}
	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	message, ok := cmd().(ui.QuickAddSubmitMsg)
	if !ok || message.Task.Description != "Final UAT task" || message.Task.Project != "work" || len(message.Task.Tags) != 1 || message.Task.Estimate == nil || message.Task.Estimate.Minutes != 90 {
		t.Fatalf("message=%#v", message)
	}
}

func TestQuickAddExplicitOnlyReachesOneMutationThroughRoot(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Width, model.Height = 80, 24
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("Discuss Friday #work")
	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model.Update(cmd())
	_, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("explicit-only command missing")
	}
	message, ok := cmd().(ui.QuickAddSubmitMsg)
	if !ok {
		t.Fatalf("message=%T", message)
	}
	_, mutation := model.Update(message)
	if mutation == nil || model.Overlay != OverlayNone || model.QuickAdd.Open {
		t.Fatalf("overlay=%s open=%v mutation=%v", model.Overlay, model.QuickAdd.Open, mutation != nil)
	}
	result := mutation().(MutationMsg)
	if result.Err != nil || len(client.mutations) != 1 || client.mutations[0].Input.Description != "Discuss Friday" {
		t.Fatalf("result=%#v mutations=%#v", result, client.mutations)
	}
}

func TestQuickAddRejectsDelayedMessagesAfterReopen(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Width, model.Height = 80, 24
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("Old capture tomorrow")
	_, oldCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model.QuickAdd.Close()
	model.Overlay = OverlayNone
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("New capture")
	_, mutation := model.Update(oldCommand())
	if mutation != nil || model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open || model.QuickAdd.Input.Value() != "New capture" {
		t.Fatalf("stale message changed current capture: overlay=%s open=%v input=%q mutation=%v", model.Overlay, model.QuickAdd.Open, model.QuickAdd.Input.Value(), mutation != nil)
	}

	model = actionModel(&fakeClient{})
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("Old capture tomorrow")
	_, oldCommand = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model.QuickAdd.Input.SetValue("Edited capture")
	if _, mutation = model.Update(oldCommand()); mutation != nil || model.QuickAdd.ReviewOpen {
		t.Fatalf("stale edited message was accepted: review=%v mutation=%v", model.QuickAdd.ReviewOpen, mutation != nil)
	}
}

func TestQuickAddUnresolvedRecurrenceConflictDoesNotMutate(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Width, model.Height = 80, 24
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("Report tomorrow every Friday")
	_, parseCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model.Update(parseCommand())
	model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	model.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	_, submitCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if submitCommand == nil {
		t.Fatal("confirmation command missing")
	}
	message, ok := submitCommand().(ui.QuickAddErrorMsg)
	if !ok || message.Err == nil {
		t.Fatalf("unresolved recurrence conflict submitted: %#v", message)
	}
	model.Update(message)
	if len(client.mutations) != 0 {
		t.Fatalf("unresolved recurrence conflict mutated: %#v", client.mutations)
	}
}

func TestQuickAddBusySubmissionPreservesDraft(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.OpenQuickAdd()
	model.MutationRunning = true
	message := ui.QuickAddSubmitMsg{Task: domain.NewTask{Description: "keep me"}, Revision: model.QuickAdd.CaptureRevision}
	_, cmd := model.Update(message)
	if cmd != nil || !model.QuickAdd.Open || model.Overlay != OverlayQuickAdd || model.QuickAdd.Input.Value() == "" && model.QuickAdd.ErrorText() == "" {
		t.Fatalf("busy submission lost draft: open=%v overlay=%s cmd=%v err=%q", model.QuickAdd.Open, model.Overlay, cmd, model.QuickAdd.ErrorText())
	}
}

func TestQuickAddAddFailureReopensConfirmedDraft(t *testing.T) {
	client := &estimateAddClient{fakeClient: &fakeClient{}, err: &taskwarrior.EstimateUDAError{State: taskwarrior.EstimateUDAMissing}}
	model := testModel(client)
	model.Width, model.Height = 80, 24
	model.OpenQuickAdd()
	model.QuickAdd.Input.SetValue("write docs ~1h")
	message := ui.QuickAddSubmitMsg{
		Task:     domain.NewTask{Description: "write docs", Estimate: &domain.Estimate{Minutes: 60}},
		Revision: model.QuickAdd.CaptureRevision, Source: "write docs ~1h", Review: true,
	}
	_, cmd := model.Update(message)
	if cmd == nil {
		t.Fatal("submission did not start mutation")
	}
	model.Update(cmd().(MutationMsg))
	if model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open || !model.QuickAdd.ReviewOpen || !strings.Contains(model.QuickAdd.View(), "estimate UDA") || len(client.mutations) != 0 {
		t.Fatalf("failed draft was not recoverable: overlay=%s open=%v review=%v view=%q mutations=%#v", model.Overlay, model.QuickAdd.Open, model.QuickAdd.ReviewOpen, model.QuickAdd.View(), client.mutations)
	}
	client.err = nil
	_, retryCommand := model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if retryCommand == nil {
		t.Fatal("recovery did not require a deliberate retry")
	}
	retryMessage, ok := retryCommand().(ui.QuickAddSubmitMsg)
	if !ok {
		t.Fatalf("retry message=%T", retryMessage)
	}
	_, retryMutation := model.Update(retryMessage)
	if retryMutation == nil {
		t.Fatal("recovery retry did not start mutation")
	}
	if result := retryMutation().(MutationMsg); result.Err != nil || len(client.mutations) != 1 {
		t.Fatalf("retry result=%#v mutations=%#v", result, client.mutations)
	}
}
