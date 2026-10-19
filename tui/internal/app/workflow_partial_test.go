package app

import (
	"errors"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

func TestPendingRenameWithZeroMatchesSavesCatalogWithoutTaskCommands(t *testing.T) {
	migrationClient := &coordinatorClient{}
	model, store := workflowModel(migrationClient)
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

// TestRenameRefusedAfterTheCatalogSaveNeedsAFreshPreview covers a migration
// the server refuses because the tasks changed: the saved catalog stays, no
// task moved, and a fresh preview can retry it (SP-15).
func TestRenameRefusedAfterTheCatalogSaveNeedsAFreshPreview(t *testing.T) {
	migrationClient := &coordinatorClient{
		tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}},
		err:   &api.Error{Code: api.CodeConflict, Message: "the tasks changed since the preview"},
	}
	store := &settingsStore{}
	model := settingsModel(store)
	model.MigrationCoordinator = NewProjectMigrationCoordinator(store, migrationClient)
	model.Client = &fakeClient{exports: [][]domain.Task{{}}}
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
	if refresh == nil || len(migrationClient.calls) != 2 || model.ProjectRename.Open || model.Undo.Available(model.now()) {
		t.Fatalf("retry: refresh %v, calls %v, rename open %v, undo %#v", refresh, migrationClient.calls, model.ProjectRename.Open, model.Undo)
	}
	model.Update(refresh())
	if model.Status != "Migration: 1 changed; refreshed 0 tasks" || model.Undo.Available(model.now()) {
		t.Fatalf("after retry: status %q, undo %#v", model.Status, model.Undo)
	}
}
