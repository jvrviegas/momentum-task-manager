package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

type settingsStore struct {
	snapshot  config.ProjectCatalogSnapshot
	saved     []domain.ProjectCatalog
	saveErr   error
	readCalls int
}

func (s *settingsStore) Read(context.Context) (config.ProjectCatalogSnapshot, error) {
	s.readCalls++
	return s.snapshot, nil
}

func (s *settingsStore) Save(_ context.Context, _ config.ProjectCatalogSnapshot, projects domain.ProjectCatalog) (config.ProjectCatalogSnapshot, error) {
	s.saved = append(s.saved, append(domain.ProjectCatalog(nil), projects...))
	if s.saveErr != nil {
		return config.ProjectCatalogSnapshot{}, s.saveErr
	}
	s.snapshot.Projects = append(domain.ProjectCatalog(nil), projects...)
	return s.snapshot, nil
}

func settingsKey(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func settingsModel(store config.ProjectCatalogStore) *Model {
	model := actionModel(&fakeClient{})
	model.ProjectStore = store
	model.Config.Projects = domain.ProjectCatalog{{Name: "Work", Value: "work"}}
	model.ProjectSettings.SetProjects(model.Config.Projects)
	model.refreshProjectSuggestions()
	model.Width = 100
	model.Height = 24
	model.ProjectSettings.SetSize(model.Width, model.Height)
	return model
}

func TestProjectCatalogSnapshotLoadsAsTypedAsyncMessage(t *testing.T) {
	store := &settingsStore{snapshot: config.ProjectCatalogSnapshot{Path: "/tmp/project-config.toml"}}
	model := settingsModel(store)
	message, ok := ProjectCatalogSnapshotCommand(context.Background(), store)().(ProjectCatalogSnapshotMsg)
	if !ok || message.Err != nil || message.Snapshot.Path != "/tmp/project-config.toml" {
		t.Fatalf("message=%#v", message)
	}
	model.Update(message)
	if model.ProjectSnapshot.Path != "/tmp/project-config.toml" || store.readCalls != 1 {
		t.Fatalf("snapshot=%#v reads=%d", model.ProjectSnapshot, store.readCalls)
	}
}

func TestSettingsRouteIsExplicitAndCannotMutateHiddenTask(t *testing.T) {
	client := &fakeClient{}
	model := actionModel(client)
	model.Width, model.Height = 79, 20
	model.Update(key("4"))
	if model.ActiveView != ViewSettings || !model.ProjectSettings.Open {
		t.Fatalf("settings route model=%#v", model)
	}
	if _, ok := model.SelectedTask(); ok {
		t.Fatal("Settings exposed a task selection")
	}
	model.Update(key(" "))
	model.Update(key("s"))
	if len(client.mutations) != 0 {
		t.Fatalf("Settings key mutated a hidden task: %#v", client.mutations)
	}
	content := model.View().Content
	if !containsAll(content, "Settings / Projects", "Settings") || strings.Contains(content, "Settings 0") {
		t.Fatalf("settings view=%q", content)
	}
}

func TestSettingsNumberShortcutsReturnToTaskViews(t *testing.T) {
	for _, test := range []struct {
		key  string
		view ViewName
	}{
		{key: "1", view: ViewInbox},
		{key: "2", view: ViewToday},
	} {
		t.Run(test.key, func(t *testing.T) {
			model := actionModel(&fakeClient{})
			model.Update(key("4"))
			model.Update(key(test.key))
			if model.ActiveView != test.view || model.ProjectSettings.Open {
				t.Fatalf("shortcut %s left active=%s settings_open=%t", test.key, model.ActiveView, model.ProjectSettings.Open)
			}
		})
	}
}

func TestSettingsMouseNavigationUsesActualCompactTabBoundary(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Width, model.Height = 79, 20
	nav := model.navigationItems()
	settingsX := -1
	for x := 0; x < model.Width; x++ {
		if ui.TabIndexAt(nav, model.Width, x) == 3 {
			settingsX = x
			break
		}
	}
	if settingsX < 0 {
		t.Fatal("settings tab was not rendered")
	}
	model.Update(tea.MouseClickMsg{X: settingsX, Y: 1, Button: tea.MouseLeft})
	if model.ActiveView != ViewSettings {
		t.Fatalf("tab click x=%d active=%s", settingsX, model.ActiveView)
	}
	inboxX := -1
	for x := 0; x < model.Width; x++ {
		if ui.TabIndexAt(nav, model.Width, x) == 0 {
			inboxX = x
			break
		}
	}
	model.Update(tea.MouseClickMsg{X: inboxX, Y: 1, Button: tea.MouseLeft})
	if model.ActiveView != ViewInbox || model.ProjectSettings.Open {
		t.Fatalf("inbox tab click x=%d active=%s settings_open=%t", inboxX, model.ActiveView, model.ProjectSettings.Open)
	}
}

func TestSettingsRoundTripPreservesTaskSelection(t *testing.T) {
	model := actionModel(&fakeClient{})
	model.Selected[ViewInbox] = "two"
	model.Selections[ViewInbox] = 1
	model.Update(key("4"))
	_, closeCmd := model.Update(key("esc"))
	if closeCmd == nil {
		t.Fatal("settings close did not emit cancel intent")
	}
	model.Update(closeCmd())
	if model.ActiveView != ViewInbox || model.Selected[ViewInbox] != "two" || model.Selections[ViewInbox] != 1 {
		t.Fatalf("round-trip lost selection: active=%s selected=%#v indexes=%#v", model.ActiveView, model.Selected, model.Selections)
	}
}

func TestCatalogSavePublishesAfterAsyncSuccessAndRefreshesSuggestions(t *testing.T) {
	store := &settingsStore{}
	model := settingsModel(store)
	model.Update(ProjectsMsg{Values: []string{"personal"}})
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsName).SetValue("Delivery")
	_, intentCmd := model.Update(settingsKey('s', tea.ModCtrl))
	if intentCmd == nil {
		t.Fatal("settings save intent was not returned")
	}
	intent, ok := intentCmd().(ui.ProjectSettingsSaveMsg)
	if !ok {
		t.Fatalf("intent=%T", intentCmd())
	}
	if len(store.saved) != 0 || model.Config.Projects[0].Name != "Work" {
		t.Fatal("save happened before async command/result")
	}
	_, saveCmd := model.Update(intent)
	if saveCmd == nil || len(store.saved) != 0 {
		t.Fatalf("save command=%v saved=%#v", saveCmd, store.saved)
	}
	result, ok := saveCmd().(ProjectCatalogSaveMsg)
	if !ok || result.Err != nil {
		t.Fatalf("result=%#v", result)
	}
	model.Update(result)
	if len(store.saved) != 1 || model.Config.Projects[0].Name != "Delivery" || model.ProjectSettings.Editing {
		t.Fatalf("save result model=%#v saved=%#v", model, store.saved)
	}
	if !reflect.DeepEqual(model.QuickAdd.Projects, []string{"work", "personal"}) || !reflect.DeepEqual(model.Editor.Projects, []string{"work", "personal"}) {
		t.Fatalf("suggestions not refreshed: quick=%v editor=%v", model.QuickAdd.Projects, model.Editor.Projects)
	}
	if len(model.Client.(*fakeClient).mutations) != 0 {
		t.Fatal("catalog save mutated tasks")
	}
}

func TestDirtySettingsQuitRequiresExplicitDiscardBeforeQuitting(t *testing.T) {
	model := settingsModel(&settingsStore{})
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsName).SetValue("Changed")
	model.Update(key("q"))
	if !model.ProjectSettings.DiscardPromptOpen || !model.QuitAfterProjectSettings {
		t.Fatalf("dirty quit skipped prompt: %#v", model)
	}
	_, discardCmd := model.Update(key("d"))
	if discardCmd == nil {
		t.Fatal("discard did not emit intent")
	}
	_, quitCmd := model.Update(discardCmd())
	if quitCmd == nil || model.ActiveView != ViewInbox || model.ProjectSettings.Editing {
		t.Fatalf("discard did not return to quit flow: active=%s model=%#v", model.ActiveView, model)
	}
}

func TestCatalogSaveFailureRetainsDraftAndLiveCatalog(t *testing.T) {
	errSave := errors.New("read-only")
	store := &settingsStore{saveErr: errSave}
	model := settingsModel(store)
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsName).SetValue("Delivery")
	_, intentCmd := model.Update(settingsKey('s', tea.ModCtrl))
	intent := intentCmd().(ui.ProjectSettingsSaveMsg)
	_, saveCmd := model.Update(intent)
	result := saveCmd().(ProjectCatalogSaveMsg)
	model.Update(result)
	if !model.ProjectSettings.Editing || model.ProjectSettings.Err != errSave || model.ProjectSettings.Input(ui.ProjectSettingsName).Value() != "Delivery" {
		t.Fatalf("failed save lost draft: %#v", model.ProjectSettings)
	}
	if model.Config.Projects[0].Name != "Work" || !reflect.DeepEqual(model.QuickAdd.Projects, []string{"work"}) {
		t.Fatalf("failed save changed live catalog: config=%#v projects=%v", model.Config.Projects, model.QuickAdd.Projects)
	}
}

func TestCatalogDiscoveryUsesSeparateSnapshotAfterConfiguredRename(t *testing.T) {
	store := &settingsStore{}
	model := settingsModel(store)
	model.MigrationCoordinator = NewProjectMigrationCoordinator(store, &coordinatorClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "discovered", Status: "pending", Project: "personal"}}},
	})
	model.Update(ProjectsMsg{Values: []string{"personal"}})
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsName).SetValue("Delivery")
	model.ProjectSettings.Input(ui.ProjectSettingsValue).SetValue("delivery")
	_, intentCmd := model.Update(settingsKey('s', tea.ModCtrl))
	intent := intentCmd().(ui.ProjectSettingsSaveMsg)
	_, previewCmd := model.Update(intent)
	model.Update(previewCmd())
	model.Update(key("enter"))
	_, confirmCmd := model.Update(key("y"))
	if confirmCmd == nil {
		t.Fatal("catalog-only rename did not emit confirmation")
	}
	_, saveCmd := model.Update(confirmCmd())
	if saveCmd == nil {
		t.Fatal("catalog-only rename did not start save")
	}
	model.Update(saveCmd())
	if !reflect.DeepEqual(model.QuickAdd.Projects, []string{"delivery", "personal"}) {
		t.Fatalf("old configured value was reintroduced as false discovery: %v", model.QuickAdd.Projects)
	}
	model.Update(ProjectsMsg{Values: []string{"work"}})
	if !reflect.DeepEqual(model.QuickAdd.Projects, []string{"delivery", "work"}) {
		t.Fatalf("new discovery merge=%v", model.QuickAdd.Projects)
	}
}

func TestStaleProjectDiscoveryResultIsIgnored(t *testing.T) {
	model := settingsModel(&settingsStore{})
	model.ProjectDiscoveryID = 2
	model.Update(ProjectsMsg{RequestID: 2, Values: []string{"current"}})
	model.Update(ProjectsMsg{RequestID: 1, Values: []string{"stale"}})
	if !reflect.DeepEqual(model.DiscoveredProjects, []string{"current"}) || !reflect.DeepEqual(model.QuickAdd.Projects, []string{"work", "current"}) {
		t.Fatalf("stale discovery applied: discovered=%v projects=%v", model.DiscoveredProjects, model.QuickAdd.Projects)
	}
}

func TestStaleCatalogSaveResultIsIgnored(t *testing.T) {
	store := &settingsStore{}
	model := settingsModel(store)
	model.SwitchView(ViewSettings)
	model.ProjectSettings.OpenEdit(0)
	model.ProjectSettings.Input(ui.ProjectSettingsName).SetValue("Delivery")
	_, intentCmd := model.Update(settingsKey('s', tea.ModCtrl))
	intent := intentCmd().(ui.ProjectSettingsSaveMsg)
	_, saveCmd := model.Update(intent)
	if !model.ProjectSaveRunning {
		t.Fatal("save gate was not acquired")
	}
	stale := ProjectCatalogSaveMsg{ID: model.ProjectSaveID + 1, Plan: intent.Plan, Snapshot: config.ProjectCatalogSnapshot{}, Err: errors.New("stale")}
	model.Update(stale)
	if !model.ProjectSaveRunning || model.Config.Projects[0].Name != "Work" {
		t.Fatal("stale result changed save state")
	}
	model.Update(saveCmd())
	if model.ProjectSaveRunning || model.Config.Projects[0].Name != "Delivery" {
		t.Fatalf("current result was not handled safely: %#v", model)
	}
}

func containsAll(value string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}
