package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/taskwarrior"
)

func lifecycleRequest(t *testing.T, migrate bool) ProjectMigrationRequest {
	t.Helper()
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	return coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, []domain.ProjectTaskMapping{mapping}, migrate)
}

func lifecycleModel(coordinator *ProjectMigrationCoordinator) *Model {
	model := actionModel(&fakeClient{exports: [][]domain.Task{{testTask("one", 1)}}})
	model.MigrationCoordinator = coordinator
	model.SyncConfigured = true
	model.SyncReady = true
	model.Mode = ModeReady
	return model
}

func TestMigrationGateBlocksCompetingTaskWritesSyncAndUndo(t *testing.T) {
	model := lifecycleModel(NewProjectMigrationCoordinator(&coordinatorStore{}, &coordinatorClient{}))
	model.MigrationRunning = true
	model.Mode = ModeMutating
	if cmd := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: "one"}); cmd != nil {
		t.Fatal("task mutation started during migration")
	}
	if cmd := model.beginSync(); cmd != nil {
		t.Fatal("sync started during migration")
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

func TestCatalogOnlyMigrationRestoresSyncStateAndDoesNotRefreshTasks(t *testing.T) {
	store := &coordinatorStore{}
	coordinator := NewProjectMigrationCoordinator(store, nil)
	model := lifecycleModel(coordinator)
	model.SyncConfigured = false
	before := model.Sync
	request := lifecycleRequest(t, false)
	cmd := model.beginProjectMigration(request)
	if cmd == nil || !model.MigrationRunning || model.Sync.UndoAvailable {
		t.Fatalf("start state=%#v cmd=%v", model, cmd)
	}
	result, ok := cmd().(ProjectMigrationMsg)
	if !ok || result.Result.CatalogErr != nil {
		t.Fatalf("result=%#v", result)
	}
	_, followUp := model.Update(result)
	if followUp != nil || model.MigrationRunning || !reflect.DeepEqual(model.Sync, before) || model.Mode != ModeReady {
		t.Fatalf("catalog-only state=%#v follow-up=%v", model, followUp)
	}
	if len(store.saved) != 1 || len(model.Tasks) != 2 {
		t.Fatalf("catalog-only side effects saved=%v tasks=%v", store.saved, model.Tasks)
	}
}

func TestMigrationTaskChangesDisableUndoMarkUnsyncedAndReleaseAfterRefresh(t *testing.T) {
	store := &coordinatorStore{}
	client := &coordinatorClient{
		export:   taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}},
		outcomes: []taskwarrior.ProjectTaskOutcome{{Kind: taskwarrior.ProjectTaskChanged}},
	}
	model := lifecycleModel(NewProjectMigrationCoordinator(store, client))
	model.Sync, _ = model.Sync.Mutation(model.now())
	model.Sync.UndoAvailable = true
	beforeTasks := append([]domain.Task(nil), model.Tasks...)
	cmd := model.beginProjectMigration(lifecycleRequest(t, true))
	result := cmd().(ProjectMigrationMsg)
	_, refresh := model.Update(result)
	if refresh == nil || !model.MigrationRunning || !model.Sync.Unsynced || model.Sync.UndoAvailable || !model.Sync.UndoUntil.IsZero() {
		t.Fatalf("migration task state=%#v refresh=%v", model, refresh)
	}
	if !reflect.DeepEqual(model.Tasks, beforeTasks) {
		t.Fatal("task list changed before migration refresh")
	}
	message, ok := refresh().(TasksMsg)
	if !ok || message.Err != nil {
		t.Fatalf("refresh message=%#v", message)
	}
	model.Update(message)
	if model.MigrationRunning || model.MigrationRefreshPending || model.Mode != ModeReady {
		t.Fatalf("migration gate not released=%#v", model)
	}
	if len(store.saved) != 1 || len(client.calls) != 1 {
		t.Fatalf("migration calls saved=%v tasks=%v", store.saved, client.calls)
	}
}

func TestMigrationPartialTaskChangeKeepsDirtyStateAndLaterOrdinaryUndo(t *testing.T) {
	store := &coordinatorStore{}
	client := &coordinatorClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{
			{UUID: "one", Status: "pending", Project: "work"},
			{UUID: "two", Status: "pending", Project: "work"},
		}},
		outcomes: []taskwarrior.ProjectTaskOutcome{{Kind: taskwarrior.ProjectTaskChanged}, {Kind: taskwarrior.ProjectTaskFailed, Err: errors.New("hook")}},
	}
	model := lifecycleModel(NewProjectMigrationCoordinator(store, client))
	request := lifecycleRequest(t, true)
	request.TaskMappings = append(request.TaskMappings, domain.ProjectTaskMapping{UUID: "two", OldValue: "work", NewValue: "delivery"})
	cmd := model.beginProjectMigration(request)
	_, refresh := model.Update(cmd().(ProjectMigrationMsg))
	if refresh == nil {
		t.Fatal("partial changed migration did not request refresh")
	}
	model.Update(refresh().(TasksMsg))
	if !model.Sync.Unsynced || model.Sync.UndoAvailable || model.MigrationRunning {
		t.Fatalf("partial state=%#v", model)
	}

	model.MutationRunning = false
	ordinary := model.beginMutation(MutationRequest{Kind: MutationComplete, UUID: "one"})
	if ordinary == nil {
		t.Fatal("ordinary mutation did not resume")
	}
	model.Update(ordinary().(MutationMsg))
	if !model.Sync.UndoAvailable {
		t.Fatalf("ordinary mutation did not restore undo grace: %#v", model.Sync)
	}
}

func TestMigrationFailureAndCancellationDoNotLosePreexistingDirtySyncState(t *testing.T) {
	before := NewSyncState(config.SyncConfig{Enabled: true, Interval: time.Minute, MutationDelay: time.Second, Startup: false, Shutdown: true}, time.Now())
	before.Unsynced = true
	before.UndoAvailable = true
	before.UndoUntil = time.Now().Add(time.Second)
	before.Phase = SyncGrace

	for _, tc := range []struct {
		name       string
		storeError error
	}{
		{name: "catalog failure", storeError: errors.New("config failure")},
		{name: "preflight cancellation", storeError: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &coordinatorStore{err: tc.storeError}
			client := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{}}
			model := lifecycleModel(NewProjectMigrationCoordinator(store, client))
			model.Sync = before
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
			if model.MigrationRunning || !reflect.DeepEqual(model.Sync, before) {
				t.Fatalf("state=%#v before=%#v", model.Sync, before)
			}
		})
	}
}

func TestSyncAndRefreshTimersDeferWhileMigrationOwnsGate(t *testing.T) {
	model := lifecycleModel(NewProjectMigrationCoordinator(&coordinatorStore{}, nil))
	model.MigrationRunning = true
	model.Sync.Phase = SyncWaiting
	now := model.now()
	model.Sync.NextAt = now
	if cmd := model.handleSyncTick(now); cmd != nil || !model.SyncDeferred {
		t.Fatalf("sync tick was not deferred: cmd=%v state=%#v", cmd, model.Sync)
	}
	model.Config.RefreshInterval = time.Millisecond
	if cmd := model.handleRefreshTick(now); cmd == nil {
		t.Fatal("refresh timer was dropped instead of rescheduled")
	}
	if cmd := model.startStartupSync(); cmd != nil {
		t.Fatal("startup sync started during migration")
	}
	model.MigrationRunning = false
	model.SyncDeferred = false
}

func TestMigrationCommandReturnsTypedResultWhenCoordinatorUnavailable(t *testing.T) {
	message, ok := ProjectMigrationCommand(context.Background(), nil, 7, lifecycleRequest(t, true))().(ProjectMigrationMsg)
	if !ok || message.ID != 7 || !errors.Is(message.Result.PreflightErr, ErrProjectMigrationNoClient) {
		t.Fatalf("message=%#v", message)
	}
}
