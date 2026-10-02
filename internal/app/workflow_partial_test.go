package app

import (
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/taskwarrior"
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
