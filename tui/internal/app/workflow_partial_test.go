package app

import (
	"errors"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

func TestPendingRenameReportsPartialTaskOutcomesAfterRefresh(t *testing.T) {
	migrationClient := &coordinatorClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{
			{UUID: "one", Status: "pending", Project: "work"},
			{UUID: "two", Status: "pending", Project: "work"},
		}},
		outcomes: []taskwarrior.ProjectTaskOutcome{
			{Kind: taskwarrior.ProjectTaskChanged},
			{Kind: taskwarrior.ProjectTaskFailed},
		},
	}
	model, store := workflowModel(migrationClient)
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	confirmMsg := confirmRename(t, model)()
	_, migrationCmd := model.Update(confirmMsg)
	result := migrationCmd().(ProjectMigrationMsg)
	_, refresh := model.Update(result)
	if refresh == nil {
		t.Fatal("partial migration did not refresh")
	}
	model.Update(refresh())
	if model.MigrationOutcome == nil || model.MigrationOutcome.ChangedCount() != 1 || model.MigrationOutcome.FailedCount() != 1 || len(store.saved) != 1 {
		t.Fatalf("partial result=%#v status=%q", model.MigrationOutcome, model.Status)
	}
	if model.ProjectSettings.Editing || model.ProjectRename.Open || model.Config.Projects[0].Value != "delivery" {
		t.Fatalf("partial workflow state=%#v", model)
	}
	if model.Status != "Migration: 1 changed, 0 skipped, 1 failed, 0 ambiguous; refreshed 0 tasks" {
		t.Fatalf("partial status=%q", model.Status)
	}
}

func TestPendingRenameWithZeroMatchesSavesCatalogWithoutTaskCommands(t *testing.T) {
	migrationClient := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{}}
	model, store := workflowModel(migrationClient)
	model.SyncConfigured = false
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	confirmMsg := confirmRename(t, model)()
	_, migrationCmd := model.Update(confirmMsg)
	result := migrationCmd().(ProjectMigrationMsg)
	_, refresh := model.Update(result)
	if refresh != nil || len(store.saved) != 1 || len(migrationClient.calls) != 0 || model.MigrationRunning {
		t.Fatalf("zero-match workflow=%#v saved=%v calls=%v refresh=%v", model, store.saved, migrationClient.calls, refresh)
	}
	if model.Config.Projects[0].Value != "delivery" || model.Status != "Projects saved; no tasks changed" {
		t.Fatalf("zero-match result=%#v status=%q", model.Config.Projects, model.Status)
	}
}

// TestBatchRenameRefusedAfterTheCatalogSaveNeedsAFreshPreview covers backend
// mode's migration in one request: when the server refuses it because the
// tasks changed, the saved catalog stays, no task moved, and a fresh preview
// can retry it (SP-15).
func TestBatchRenameRefusedAfterTheCatalogSaveNeedsAFreshPreview(t *testing.T) {
	migrationClient := &batchClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}},
		err:    &api.Error{Code: api.CodeConflict, Message: "the tasks changed since the preview"},
	}
	store := &settingsStore{}
	model := settingsModel(store)
	model.MigrationCoordinator = NewProjectMigrationCoordinator(store, migrationClient)
	model.Client = &fakeClient{exports: [][]domain.Task{{}}}
	model.Backend = true
	model.Sync = NewBackendSyncState(15 * time.Second)
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	_, migrationCmd := model.Update(confirmRename(t, model)())
	_, refresh := model.Update(migrationCmd().(ProjectMigrationMsg))
	if refresh != nil || len(store.saved) != 1 || len(migrationClient.calls) != 1 || model.MigrationRunning || model.Config.Projects[0].Value != "delivery" {
		t.Fatalf("refused: refresh %v, saved %v, calls %v, running %v", refresh, store.saved, migrationClient.calls, model.MigrationRunning)
	}
	if !model.ProjectRename.Open || model.ProjectRename.PreviewValid || !errors.Is(model.ProjectRename.Err, ErrProjectMigrationStale) ||
		model.Status != "Projects saved; no tasks moved because the preview is stale; review it again" {
		t.Fatalf("refused: rename %#v, status %q", model.ProjectRename, model.Status)
	}

	migrationClient.err = nil
	_, optionCmd := model.Update(key("r"))
	_, previewCmd := model.Update(optionCmd())
	model.Update(previewCmd())
	_, migrationCmd = model.Update(confirmRename(t, model)())
	_, refresh = model.Update(migrationCmd().(ProjectMigrationMsg))
	if refresh == nil || len(migrationClient.calls) != 2 || model.ProjectRename.Open || model.Sync.UndoAvailable {
		t.Fatalf("retry: refresh %v, calls %v, rename open %v, sync %#v", refresh, migrationClient.calls, model.ProjectRename.Open, model.Sync)
	}
	model.Update(refresh())
	if model.Status != "Migration: 1 changed, 0 skipped, 0 failed, 0 ambiguous; refreshed 0 tasks" || model.Sync.UndoAvailable {
		t.Fatalf("after retry: status %q, sync %#v", model.Status, model.Sync)
	}
}
