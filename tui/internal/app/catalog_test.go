package app

import (
	"context"
	"testing"
)

type optionalClient struct{ *fakeClient }

func (c optionalClient) Projects(context.Context) ([]string, error)   { return []string{"work"}, nil }
func (c optionalClient) Tags(context.Context) ([]string, error)       { return []string{"planning"}, nil }
func (c optionalClient) Context(context.Context) (string, error)      { return "work", nil }
func (c optionalClient) SyncConfigured(context.Context) (bool, error) { return false, nil }

func TestContextMessageIsShownWithoutChangingIt(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Update(ContextMsg{Name: "work"})
	if model.TaskContext != "work" {
		t.Fatalf("context=%q", model.TaskContext)
	}
	model.Update(ContextMsg{Err: context.Canceled})
	if model.TaskContext != "work" {
		t.Fatalf("context changed on error=%q", model.TaskContext)
	}
}

func TestCatalogMessagesUpdateBothInputOverlays(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Update(ProjectsMsg{Values: []string{"work"}})
	model.Update(TagsMsg{Values: []string{"planning"}})
	if len(model.QuickAdd.Projects) != 1 || len(model.QuickAdd.Tags) != 1 || len(model.Editor.Projects) != 1 || len(model.Editor.Tags) != 1 {
		t.Fatalf("quickadd=%#v editor=%#v", model.QuickAdd, model.Editor)
	}
}

func TestOptionalCatalogCommandsAreScheduledOnOverlayOpen(t *testing.T) {
	client := optionalClient{fakeClient: &fakeClient{}}
	model := actionModel(client)
	if cmd := model.OpenQuickAdd(); cmd == nil || model.Overlay != OverlayQuickAdd {
		t.Fatal("quick-add did not schedule catalog loading")
	}
	model.QuickAdd.Close()
	model.Overlay = OverlayNone
	if cmd := model.OpenEditor(0); cmd == nil || model.Overlay != OverlayEdit {
		t.Fatal("editor did not schedule catalog loading")
	}
}

func TestContextCommandReturnsTypedMessage(t *testing.T) {
	message, ok := ContextCommand(context.Background(), optionalClient{fakeClient: &fakeClient{}})().(ContextMsg)
	if !ok || message.Err != nil || message.Name != "work" {
		t.Fatalf("message=%#v", message)
	}
}

func TestSearchFilteringStillUsesTaskIdentity(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Search.Active = true
	model.Search.Query = "one"
	model.Selections[ViewInbox] = 0
	model.Selected[ViewInbox] = "one"
	if task, ok := model.SelectedTask(); !ok || task.UUID != "one" {
		t.Fatalf("selected=%#v ok=%v", task, ok)
	}
}
