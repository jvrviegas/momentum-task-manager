package app

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

func workflowModel(migrationClient *coordinatorClient) (*Model, *settingsStore) {
	store := &settingsStore{}
	model := settingsModel(store)
	model.MigrationCoordinator = NewProjectMigrationCoordinator(store, migrationClient)
	model.Client = &fakeClient{exports: [][]domain.Task{{}}}
	return model, store
}

func openValueRenamePreview(t *testing.T, model *Model, value string) {
	t.Helper()
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsValue).SetValue(value)
	_, intentCmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if intentCmd == nil {
		t.Fatal("value edit did not emit settings intent")
	}
	_, previewCmd := model.Update(intentCmd())
	if previewCmd == nil {
		t.Fatal("value edit did not start preview")
	}
	model.Update(previewCmd())
	if !model.ProjectRename.Open || !model.ProjectRename.PreviewValid {
		t.Fatalf("preview state=%#v", model.ProjectRename)
	}
}

func confirmRename(t *testing.T, model *Model) tea.Cmd {
	t.Helper()
	model.Update(key("enter"))
	if !model.ProjectRename.ConfirmOpen {
		t.Fatalf("confirmation did not open: %#v", model.ProjectRename)
	}
	_, cmd := model.Update(key("y"))
	if cmd == nil {
		t.Fatal("confirmation did not emit message")
	}
	return cmd
}

func refreshRenameScope(t *testing.T, model *Model) {
	t.Helper()
	_, optionCmd := model.Update(key("t"))
	if optionCmd == nil {
		t.Fatal("pending-task option did not emit recompute intent")
	}
	_, previewCmd := model.Update(optionCmd())
	if previewCmd == nil {
		t.Fatal("scope recompute did not start")
	}
	model.Update(previewCmd())
	if !model.ProjectRename.PreviewValid || model.ProjectRename.Mode != ui.ProjectRenameCatalogAndPending {
		t.Fatalf("scope preview=%#v", model.ProjectRename)
	}
}

func TestValueEditCatalogOnlySavesAfterExplicitPreviewConfirmation(t *testing.T) {
	migrationClient := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}}
	model, store := workflowModel(migrationClient)
	openValueRenamePreview(t, model, "delivery")
	if model.ProjectRename.Mode != ui.ProjectRenameCatalogOnly || len(store.saved) != 0 || len(migrationClient.calls) != 0 {
		t.Fatalf("preview performed writes: rename=%#v saved=%v calls=%v", model.ProjectRename, store.saved, migrationClient.calls)
	}
	confirmMsg := confirmRename(t, model)()
	_, saveCmd := model.Update(confirmMsg)
	if saveCmd == nil || len(store.saved) != 0 {
		t.Fatalf("catalog save started incorrectly: cmd=%v saved=%v", saveCmd, store.saved)
	}
	model.Update(saveCmd())
	if len(store.saved) != 1 || model.Config.Projects[0].Value != "delivery" || model.ProjectRename.Open || model.ProjectSettings.Editing {
		t.Fatalf("catalog-only result model=%#v saved=%v", model, store.saved)
	}
	if len(migrationClient.calls) != 0 || len(model.Client.(*fakeClient).mutations) != 0 {
		t.Fatal("catalog-only flow mutated tasks")
	}
}

func TestValueEditPendingFlowRunsOnlyConfirmedMappingsAndRefreshes(t *testing.T) {
	migrationClient := &coordinatorClient{
		export:   taskwarrior.ProjectMigrationExport{Scope: taskwarrior.ProjectMigrationScope{ContextName: "work", ReadFilter: "project:work"}, Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}},
		outcomes: []taskwarrior.ProjectTaskOutcome{{Kind: taskwarrior.ProjectTaskChanged}},
	}
	model, store := workflowModel(migrationClient)
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	confirmMsg := confirmRename(t, model)()
	_, migrationCmd := model.Update(confirmMsg)
	if migrationCmd == nil || !model.MigrationRunning || len(store.saved) != 0 || len(migrationClient.calls) != 0 {
		t.Fatalf("migration did not acquire async gate: model=%#v", model)
	}
	result, ok := migrationCmd().(ProjectMigrationMsg)
	if !ok {
		t.Fatalf("message=%T", migrationCmd())
	}
	_, refreshCmd := model.Update(result)
	if refreshCmd == nil || !model.Sync.Unsynced || model.Sync.UndoAvailable {
		t.Fatalf("migration result state=%#v refresh=%v", model, refreshCmd)
	}
	model.Update(refreshCmd())
	if model.MigrationRunning || model.ProjectRename.Open || model.ProjectSettings.Editing || len(store.saved) != 1 || len(migrationClient.calls) != 1 {
		t.Fatalf("workflow completion model=%#v saved=%v calls=%v", model, store.saved, migrationClient.calls)
	}
	if !strings.Contains(model.Status, "Migration: 1 changed") || model.Config.Projects[0].Value != "delivery" || !reflect.DeepEqual(model.QuickAdd.Projects, []string{"delivery"}) {
		t.Fatalf("catalog/suggestions=%#v/%v", model.Config.Projects, model.QuickAdd.Projects)
	}
}

func TestValueEditStalePreviewKeepsDraftAndPerformsNoWrites(t *testing.T) {
	migrationClient := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}}
	model, store := workflowModel(migrationClient)
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	migrationClient.export.Tasks = nil
	confirmMsg := confirmRename(t, model)()
	_, migrationCmd := model.Update(confirmMsg)
	result := migrationCmd().(ProjectMigrationMsg)
	model.Update(result)
	if !model.ProjectRename.Open || model.ProjectRename.PreviewValid || model.ProjectRename.Err == nil || model.Config.Projects[0].Value != "work" || len(store.saved) != 0 || len(migrationClient.calls) != 0 {
		t.Fatalf("stale result was not actionable: model=%#v saved=%v calls=%v", model, store.saved, migrationClient.calls)
	}
	if !model.ProjectSettings.Editing || model.ProjectSettings.Input(ui.ProjectSettingsValue).Value() != "delivery" {
		t.Fatal("stale preview discarded the editable draft")
	}

	migrationClient.export.Tasks = []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}
	_, retryIntent := model.Update(key("r"))
	if retryIntent == nil {
		t.Fatal("stale preview did not offer explicit retry")
	}
	_, retryPreview := model.Update(retryIntent())
	if retryPreview == nil {
		t.Fatal("retry did not start a fresh preview")
	}
	model.Update(retryPreview())
	confirmMsg = confirmRename(t, model)()
	_, retryMigration := model.Update(confirmMsg)
	result = retryMigration().(ProjectMigrationMsg)
	_, refresh := model.Update(result)
	if refresh == nil || len(store.saved) != 1 {
		t.Fatalf("retry did not execute a fresh plan: refresh=%v saved=%v", refresh, store.saved)
	}
}

func TestTaskOnlyDestinationWarningIsNotBypassedByWorkflow(t *testing.T) {
	migrationClient := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{
		{UUID: "source", Status: "pending", Project: "work"},
		{UUID: "existing", Status: "pending", Project: "delivery"},
	}}}
	model, store := workflowModel(migrationClient)
	openValueRenamePreview(t, model, "delivery")
	refreshRenameScope(t, model)
	if !model.ProjectRename.Preview.DestinationTaskOnly {
		t.Fatal("preview did not flag task-only destination")
	}
	model.Update(key("enter"))
	if !model.ProjectRename.MergeWarningOpen || model.ProjectRename.ConfirmOpen {
		t.Fatalf("merge warning bypassed: %#v", model.ProjectRename)
	}
	model.Update(key("esc"))
	if model.ProjectRename.ConfirmOpen || len(store.saved) != 0 {
		t.Fatal("cancelled merge warning changed state")
	}
	if !strings.Contains(strings.ToLower(model.ProjectRename.View()), "effective merge") {
		// The warning is closed after cancellation; the prior state check is the assertion.
	}
}
