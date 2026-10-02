package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

type themeSettingsStore struct {
	settingsStore
	themes   []string
	themeErr error
}

func (s *themeSettingsStore) SaveTheme(_ context.Context, _ config.ProjectCatalogSnapshot, theme string) (config.ProjectCatalogSnapshot, error) {
	s.themes = append(s.themes, theme)
	if s.themeErr != nil {
		return config.ProjectCatalogSnapshot{}, s.themeErr
	}
	s.snapshot.Path = "/tmp/momentum-theme.toml"
	return s.snapshot, nil
}

func appearanceModel(store config.ProjectCatalogStore) *Model {
	model := settingsModel(store)
	model.SwitchView(ViewSettings)
	model.Update(settingsKey(tea.KeyTab, 0))
	return model
}

func TestSettingsTabSwitchesBetweenProjectsAndAppearance(t *testing.T) {
	model := appearanceModel(&themeSettingsStore{})
	if model.SettingsSection != SettingsAppearance || !strings.Contains(model.View().Content, "Settings / Appearance") {
		t.Fatalf("tab did not open Appearance: section=%q", model.SettingsSection)
	}
	model.Update(settingsKey(tea.KeyTab, 0))
	if model.SettingsSection != SettingsProjects || !strings.Contains(model.View().Content, "Settings / Projects") {
		t.Fatalf("tab did not return to Projects: section=%q", model.SettingsSection)
	}
}

func TestAppearanceArrowsPreviewThemeLiveAcrossComponents(t *testing.T) {
	model := appearanceModel(&themeSettingsStore{})
	if model.Styles.Theme.Name != "dark" {
		t.Fatalf("start theme=%q", model.Styles.Theme.Name)
	}
	// auto → dark → light → terminal
	for range 3 {
		model.Update(settingsKey(tea.KeyRight, 0))
	}
	if model.Appearance.Draft != config.ThemeTerminal || model.Styles.Theme.Name != "terminal" {
		t.Fatalf("draft=%q theme=%q", model.Appearance.Draft, model.Styles.Theme.Name)
	}
	if model.QuickAdd.Styles != model.Styles || model.Editor.Styles != model.Styles || model.ProjectSettings.Styles != model.Styles || model.Planner.Styles != model.Styles {
		t.Fatal("preview did not reach every component")
	}
	if model.Config.Theme != config.ThemeAuto {
		t.Fatalf("preview changed the saved config: %q", model.Config.Theme)
	}
}

func TestAppearanceEscDiscardsPreviewThenCloses(t *testing.T) {
	model := appearanceModel(&themeSettingsStore{})
	model.Update(settingsKey(tea.KeyRight, 0))
	model.Update(settingsKey(tea.KeyRight, 0))
	if model.Styles.Theme.Name != "light" {
		t.Fatalf("preview theme=%q", model.Styles.Theme.Name)
	}
	model.Update(settingsKey(tea.KeyEscape, 0))
	if model.Appearance.Dirty() || model.Styles.Theme.Name != "dark" || model.ActiveView != ViewSettings {
		t.Fatalf("esc did not discard in place: dirty=%t theme=%q view=%s", model.Appearance.Dirty(), model.Styles.Theme.Name, model.ActiveView)
	}
	_, closeCmd := model.Update(settingsKey(tea.KeyEscape, 0))
	if closeCmd == nil {
		t.Fatal("clean esc did not emit close")
	}
	model.Update(closeCmd())
	if model.ActiveView != ViewInbox || model.SettingsSection != SettingsProjects {
		t.Fatalf("close left view=%s section=%q", model.ActiveView, model.SettingsSection)
	}
}

func TestDirtyAppearanceBlocksLeavingUntilSavedOrDiscarded(t *testing.T) {
	model := appearanceModel(&themeSettingsStore{})
	model.Update(settingsKey(tea.KeyRight, 0))
	for _, press := range []tea.KeyPressMsg{key("1"), settingsKey(tea.KeyTab, 0), key("q")} {
		_, cmd := model.Update(press)
		if model.ActiveView != ViewSettings || model.SettingsSection != SettingsAppearance || cmd != nil {
			t.Fatalf("%q left a dirty draft: view=%s section=%q", press.String(), model.ActiveView, model.SettingsSection)
		}
		if !strings.Contains(model.Status, "theme change") {
			t.Fatalf("%q status=%q", press.String(), model.Status)
		}
	}
}

func TestAppearanceSavePersistsThroughStoreAndUpdatesSnapshot(t *testing.T) {
	store := &themeSettingsStore{}
	model := appearanceModel(store)
	for range 3 {
		model.Update(settingsKey(tea.KeyRight, 0))
	}
	_, intentCmd := model.Update(settingsKey(tea.KeyEnter, 0))
	if intentCmd == nil {
		t.Fatal("enter did not emit a save intent")
	}
	intent, ok := intentCmd().(ui.AppearanceSaveMsg)
	if !ok || intent.Theme != config.ThemeTerminal || len(store.themes) != 0 {
		t.Fatalf("intent=%#v saved=%v", intent, store.themes)
	}
	_, saveCmd := model.Update(intent)
	if saveCmd == nil || !model.Appearance.Saving {
		t.Fatal("save did not start asynchronously")
	}
	result, ok := saveCmd().(ThemeSaveMsg)
	if !ok || result.Err != nil {
		t.Fatalf("result=%#v", result)
	}
	model.Update(result)
	if model.Config.Theme != config.ThemeTerminal || model.Appearance.Dirty() || model.Appearance.Saving || model.Status != "Theme saved" {
		t.Fatalf("config=%q appearance=%#v status=%q", model.Config.Theme, model.Appearance, model.Status)
	}
	if model.ProjectSnapshot.Path != "/tmp/momentum-theme.toml" {
		t.Fatalf("snapshot not refreshed for later catalog saves: %#v", model.ProjectSnapshot)
	}
	model.Update(settingsKey(tea.KeyTab, 0))
	if model.SettingsSection != SettingsProjects || model.Styles.Theme.Name != "terminal" {
		t.Fatalf("saved theme did not stay applied: section=%q theme=%q", model.SettingsSection, model.Styles.Theme.Name)
	}
}

func TestAppearanceSaveFailureKeepsDraftAndPreview(t *testing.T) {
	store := &themeSettingsStore{themeErr: errors.New("read-only")}
	model := appearanceModel(store)
	model.Update(settingsKey(tea.KeyRight, 0))
	model.Update(settingsKey(tea.KeyRight, 0))
	_, intentCmd := model.Update(settingsKey(tea.KeyEnter, 0))
	_, saveCmd := model.Update(intentCmd())
	model.Update(saveCmd())
	if model.Config.Theme != config.ThemeAuto || model.Appearance.Draft != config.ThemeLight || model.Appearance.Err == nil || model.Styles.Theme.Name != "light" {
		t.Fatalf("config=%q appearance=%#v theme=%q", model.Config.Theme, model.Appearance, model.Styles.Theme.Name)
	}
	if !strings.Contains(model.Status, "Theme save failed") || !strings.Contains(model.View().Content, "read-only") {
		t.Fatalf("status=%q", model.Status)
	}
}

func TestAppearanceSaveWithoutThemeStoreReportsError(t *testing.T) {
	model := appearanceModel(&settingsStore{})
	model.Update(settingsKey(tea.KeyRight, 0))
	_, intentCmd := model.Update(settingsKey(tea.KeyEnter, 0))
	_, saveCmd := model.Update(intentCmd())
	model.Update(saveCmd())
	if model.Appearance.Err == nil || !model.Appearance.Dirty() {
		t.Fatalf("appearance=%#v", model.Appearance)
	}
}
