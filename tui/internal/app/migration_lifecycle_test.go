package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

func lifecycleRequest(t *testing.T, migrate bool) ProjectMigrationRequest {
	t.Helper()
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	return coordinatorRequest(plan, []domain.ProjectTaskMapping{mapping}, migrate)
}

func lifecycleModel(coordinator *ProjectMigrationCoordinator) *Model {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}})
	model.MigrationCoordinator = coordinator
	model.Mode = ModeReady
	return model
}

func TestMigrationGateBlocksCompetingTaskWritesAndUndo(t *testing.T) {
	model := lifecycleModel(NewProjectMigrationCoordinator(&coordinatorStore{}, &coordinatorClient{}))
	model.MigrationRunning = true
	model.Mode = ModeMutating
	model.Undo = model.Undo.Open(model.now())
	if cmd := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: "one"}); cmd != nil {
		t.Fatal("task mutation started during migration")
	}
	if cmd := model.UndoLast(); cmd != nil {
		t.Fatal("undo started during migration")
	}
	if cmd := model.requestQuit(); cmd != nil {
		t.Fatal("quit was not blocked during migration")
	}
	if model.PendingMutation != nil || model.MigrationRunning != true {
		t.Fatalf("gate state=%#v", model)
	}
}

func TestCatalogOnlyMigrationRestoresUndoAndDoesNotRefreshTasks(t *testing.T) {
	store := &coordinatorStore{}
	coordinator := NewProjectMigrationCoordinator(store, nil)
	model := lifecycleModel(coordinator)
	model.Undo = model.Undo.Open(model.now())
	before := model.Undo
	request := lifecycleRequest(t, false)
	cmd := model.beginProjectMigration(request)
	if cmd == nil || !model.MigrationRunning || model.Undo.Available(model.now()) {
		t.Fatalf("start state=%#v cmd=%v", model, cmd)
	}
	result, ok := cmd().(ProjectMigrationMsg)
	if !ok || result.Result.CatalogErr != nil {
		t.Fatalf("result=%#v", result)
	}
	_, followUp := model.Update(result)
	if model.MigrationRunning || model.Undo != before || model.Mode != ModeReady {
		t.Fatalf("catalog-only state=%#v follow-up=%v", model, followUp)
	}
	if len(store.saved) != 1 || len(model.Tasks) != 2 {
		t.Fatalf("catalog-only side effects saved=%v tasks=%v", store.saved, model.Tasks)
	}
}

func TestMigrationTaskChangesCloseUndoAndReleaseAfterRefresh(t *testing.T) {
	store := &coordinatorStore{}
	client := &coordinatorClient{tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}
	model := lifecycleModel(NewProjectMigrationCoordinator(store, client))
	model.Undo = model.Undo.Open(model.now())
	beforeTasks := append([]domain.Task(nil), model.Tasks...)
	cmd := model.beginProjectMigration(lifecycleRequest(t, true))
	result := cmd().(ProjectMigrationMsg)
	_, refresh := model.Update(result)
	if refresh == nil || !model.MigrationRunning || model.Undo.Available(model.now()) {
		t.Fatalf("migration task state=%#v refresh=%v", model, refresh)
	}
	if len(model.Tasks) != len(beforeTasks) || model.Tasks[0].UUID != beforeTasks[0].UUID {
		t.Fatal("task list changed before migration refresh")
	}
	message, ok := refresh().(TasksMsg)
	if !ok || message.Err != nil {
		t.Fatalf("refresh message=%#v", message)
	}
	model.Update(message)
	if model.MigrationRunning || model.MigrationRefreshPending || model.Mode != ModeReady || model.Undo.Available(model.now()) {
		t.Fatalf("migration gate not released=%#v", model)
	}
	if len(store.saved) != 1 || len(client.calls) != 1 || model.Status != "Migration: 1 changed; refreshed 1 tasks" {
		t.Fatalf("migration calls saved=%v tasks=%v status=%q", store.saved, client.calls, model.Status)
	}

	ordinary := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: "one"})
	if ordinary == nil {
		t.Fatal("ordinary mutation did not resume")
	}
	model.Update(ordinary().(MutationMsg))
	if !model.Undo.Available(model.now()) {
		t.Fatalf("ordinary mutation did not open undo: %#v", model.Undo)
	}
}

func TestMigrationFailureAndCancellationKeepTheUndoWindow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		storeError error
	}{
		{name: "catalog failure", storeError: errors.New("config failure")},
		{name: "preflight cancellation", storeError: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &coordinatorStore{err: tc.storeError}
			model := lifecycleModel(NewProjectMigrationCoordinator(store, &coordinatorClient{}))
			before := UndoWindow{Delay: time.Second}.Open(model.now())
			model.Undo = before
			request := lifecycleRequest(t, tc.storeError == nil)
			ctx := context.Background()
			if tc.storeError == nil {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelCtx
			}
			model.ctx = ctx
			cmd := model.beginProjectMigration(request)
			result := cmd().(ProjectMigrationMsg)
			model.Update(result)
			if model.MigrationRunning || model.Undo != before {
				t.Fatalf("state=%#v before=%#v", model.Undo, before)
			}
		})
	}
}

func TestTimersWaitWhileMigrationOwnsGate(t *testing.T) {
	model := lifecycleModel(NewProjectMigrationCoordinator(&coordinatorStore{}, nil))
	model.MigrationRunning = true
	model.Undo = model.Undo.Open(model.now())
	if cmd := model.scheduleUndoTick(); cmd != nil {
		t.Fatal("undo tick scheduled during migration")
	}
	model.Config.RefreshInterval = time.Millisecond
	if cmd := model.handleRefreshTick(model.now()); cmd == nil || model.Mode == ModeRefreshing {
		t.Fatalf("refresh timer ran or was dropped during migration: mode=%s", model.Mode)
	}
}

func TestMigrationCommandReturnsTypedResultWhenCoordinatorUnavailable(t *testing.T) {
	message, ok := ProjectMigrationCommand(context.Background(), nil, 7, lifecycleRequest(t, true))().(ProjectMigrationMsg)
	if !ok || message.ID != 7 || !errors.Is(message.Result.PreflightErr, ErrProjectMigrationNoClient) {
		t.Fatalf("message=%#v", message)
	}
}
