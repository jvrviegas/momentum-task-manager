package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
)

type fakeClient struct {
	exports    [][]domain.Task
	exportErr  error
	mutations  []MutationRequest
	syncs      int
	projects   []string
	tags       []string
	completeCh chan struct{}
}

func (f *fakeClient) ExportPending(context.Context) ([]domain.Task, error) {
	if f.completeCh != nil {
		<-f.completeCh
	}
	if len(f.exports) == 0 {
		return nil, f.exportErr
	}
	result := f.exports[0]
	f.exports = f.exports[1:]
	return result, f.exportErr
}
func (f *fakeClient) Add(_ context.Context, input domain.NewTask) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationAdd, Input: input})
	return nil
}
func (f *fakeClient) Modify(_ context.Context, uuid string, diff domain.TaskDiff) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationModify, UUID: uuid, Diff: diff})
	return nil
}
func (f *fakeClient) Complete(_ context.Context, uuid string) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationComplete, UUID: uuid})
	return nil
}
func (f *fakeClient) Delete(_ context.Context, uuid string) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationDelete, UUID: uuid})
	return nil
}
func (f *fakeClient) Start(_ context.Context, uuid string) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationStart, UUID: uuid})
	return nil
}
func (f *fakeClient) Stop(_ context.Context, uuid string) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationStop, UUID: uuid})
	return nil
}
func (f *fakeClient) Undo(_ context.Context) error {
	f.mutations = append(f.mutations, MutationRequest{Kind: MutationUndo})
	return nil
}
func (f *fakeClient) Sync(context.Context) (taskwarrior.SyncResult, error) {
	f.syncs++
	return taskwarrior.SyncResult{}, nil
}
func (f *fakeClient) Projects(context.Context) ([]string, error) { return f.projects, nil }
func (f *fakeClient) Tags(context.Context) ([]string, error)     { return f.tags, nil }

func testTask(uuid string, urgency float64) domain.Task {
	return domain.Task{UUID: uuid, Description: uuid, Status: "pending", Urgency: urgency}
}

func testModel(client taskwarrior.Client) *Model {
	settings := config.Defaults()
	return NewModel(ModelOptions{
		Client:      client,
		Config:      settings,
		InitialView: ViewAuto,
		Now:         func() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC) },
		Context:     context.Background(),
	})
}

func key(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func TestInitReturnsAsyncLoadCommand(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}}
	model := testModel(client)
	if model.Mode != ModeLoading {
		t.Fatalf("initial mode=%s", model.Mode)
	}
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("expected load command")
	}
	message, ok := cmd().(TasksMsg)
	if !ok || message.Err != nil || len(message.Tasks) != 1 {
		t.Fatalf("message=%#v", message)
	}
	model.Update(message)
	if model.Mode != ModeReady || len(model.Tasks) != 1 {
		t.Fatalf("model=%#v", model)
	}
}

func TestAutomaticStartupChoosesTodayWhenNonEmpty(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{
		testTask("inbox", 1),
		func() domain.Task {
			due := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			return domain.Task{UUID: "today", Description: "today", Status: "pending", Due: &due, Urgency: 2}
		}(),
	}}}
	model := testModel(client)
	message := clientExportMessage(client)
	model.Update(message)
	if model.ActiveView != ViewToday || len(model.Views.Today) != 1 {
		t.Fatalf("active=%s views=%#v", model.ActiveView, model.Views)
	}
}

func clientExportMessage(client *fakeClient) TasksMsg {
	tasks, err := client.ExportPending(context.Background())
	return TasksMsg{Tasks: tasks, Err: err, Reason: "initial"}
}

func TestExplicitStartupViewOverridesAutomaticChoice(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{
		func() domain.Task {
			due := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			return domain.Task{UUID: "today", Status: "pending", Due: &due}
		}(),
	}}}
	model := NewModel(ModelOptions{Client: client, Config: config.Defaults(), InitialView: ViewInbox, Now: func() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC) }})
	model.Update(clientExportMessage(client))
	if model.ActiveView != ViewInbox {
		t.Fatalf("active=%s", model.ActiveView)
	}
}

func TestRefreshKeepsVisibleContentOnError(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}}
	model := testModel(client)
	model.Update(clientExportMessage(client))
	_, cmd := model.Update(RefreshRequestedMsg{Reason: "manual"})
	if cmd == nil || model.Mode != ModeRefreshing || len(model.Tasks) != 1 {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
	client.exportErr = errors.New("task unavailable")
	model.Update(TasksMsg{Err: client.exportErr, Reason: "manual"})
	if model.Mode != ModeReady || len(model.Tasks) != 1 || model.Err == nil {
		t.Fatalf("stale content lost: %#v", model)
	}
}

func TestMutationRequestsAreSerializedAndRefreshOnceAfterSuccess(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}}
	model := testModel(client)
	model.Update(clientExportMessage(client))
	request := MutationRequest{Kind: MutationComplete, UUID: "one"}
	_, first := model.Update(MutationRequestedMsg{Request: request})
	if first == nil || !model.MutationRunning || model.Mode != ModeMutating {
		t.Fatalf("first mutation model=%#v cmd=%v", model, first)
	}
	_, second := model.Update(MutationRequestedMsg{Request: request})
	if second != nil || len(client.mutations) != 0 {
		t.Fatal("second mutation should be blocked while first is running")
	}
	result := first().(MutationMsg)
	if result.Err != nil || len(client.mutations) != 1 {
		t.Fatalf("result=%#v mutations=%#v", result, client.mutations)
	}
	_, refresh := model.Update(result)
	if refresh == nil || model.MutationRunning || model.Mode != ModeRefreshing {
		t.Fatalf("post mutation model=%#v refresh=%v", model, refresh)
	}
}

func TestFailedMutationPreservesReadyStateAndDoesNotRefresh(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Tasks = []domain.Task{testTask("one", 1)}
	model.Mode = ModeMutating
	model.MutationRunning = true
	_, cmd := model.Update(MutationMsg{Kind: MutationDelete, UUID: "one", Err: errors.New("hook failed")})
	if cmd != nil || model.Mode != ModeReady || model.MutationRunning || model.Err == nil {
		t.Fatalf("model=%#v cmd=%v", model, cmd)
	}
}

func TestOverlayOwnsInputBeforeGlobalKeys(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Overlay = OverlayQuickAdd
	model.Mode = ModeReady
	_, cmd := model.Update(key("r"))
	if cmd != nil || model.Mode != ModeReady || model.Overlay != OverlayQuickAdd {
		t.Fatalf("overlay intercepted incorrectly: %#v", model)
	}
	model.Update(key("esc"))
	if model.Overlay != OverlayNone {
		t.Fatalf("overlay=%s", model.Overlay)
	}
}

func TestWindowSizeAndNavigationTransitions(t *testing.T) {
	model := testModel(&fakeClient{})
	model.Tasks = []domain.Task{testTask("a", 3), testTask("b", 2)}
	model.Views = domain.BuildViews(model.Tasks, model.now())
	model.Mode = ModeReady
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.Update(key("j"))
	if model.Width != 100 || model.Height != 30 || model.Selected[ViewInbox] != "b" {
		t.Fatalf("model=%#v", model)
	}
	model.Update(key("2"))
	if model.ActiveView != ViewToday {
		t.Fatalf("active=%s", model.ActiveView)
	}
}

func TestCommandFactoriesReturnTypedMessages(t *testing.T) {
	client := &fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}}
	if _, ok := LoadTasksCommand(context.Background(), client, "test")().(TasksMsg); !ok {
		t.Fatal("load command returned wrong message")
	}
	if _, ok := MutationCommand(context.Background(), client, MutationRequest{Kind: MutationAdd, Input: domain.NewTask{Description: "x"}})().(MutationMsg); !ok {
		t.Fatal("mutation command returned wrong message")
	}
	if !reflect.DeepEqual(client.mutations[0].Input.Description, "x") {
		t.Fatal("mutation input not passed")
	}
}
