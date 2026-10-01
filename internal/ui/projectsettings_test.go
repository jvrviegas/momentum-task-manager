package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

func projectSettingsKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func projectSettingsSpecial(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func testProjectSettings() ProjectSettingsModel {
	model := NewProjectSettings(NewStyles(ResolveTheme("dark", true)))
	model.SetSize(70, 20)
	model.OpenProjects(domain.ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
		{Name: "Personal", Value: "personal"},
	})
	return model
}

func saveIntent(t *testing.T, cmd tea.Cmd) ProjectSettingsSaveMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	message, ok := cmd().(ProjectSettingsSaveMsg)
	if !ok {
		t.Fatalf("message=%T", cmd())
	}
	return message
}

func TestProjectSettingsRendersConfiguredHierarchyAndEmptyState(t *testing.T) {
	model := testProjectSettings()
	view := sanitizeComponentRender(model.View())
	for _, want := range []string{"Settings / Projects", "3 projects", "Work", "└ Client", "#work.client", "a add", "e edit", "d remove"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %s", want, view)
		}
	}

	model.OpenProjects(nil)
	view = sanitizeComponentRender(model.View())
	if !strings.Contains(view, "No configured projects") || !strings.Contains(view, "a to add") {
		t.Fatalf("empty view=%s", view)
	}
}

func TestProjectSettingsKeepsSelectedLongCatalogEntryVisible(t *testing.T) {
	model := NewProjectSettings(Styles{})
	model.SetSize(70, 10)
	projects := make(domain.ProjectCatalog, 20)
	for i := range projects {
		projects[i] = domain.Project{Name: "Project " + string(rune('A'+i)), Value: "project-" + string(rune('a'+i))}
	}
	model.OpenProjects(projects)
	for i := 0; i < 15; i++ {
		model.Update(projectSettingsKey("down"))
	}
	view := sanitizeComponentRender(model.View())
	if !strings.Contains(view, "Project P") {
		t.Fatalf("selected entry hidden: %s", view)
	}
	if lipgloss.Width(view) > 70 {
		t.Fatalf("view width=%d", lipgloss.Width(view))
	}
}

func TestProjectSettingsAddComposesParentAndPublishesOnlyAfterSave(t *testing.T) {
	model := testProjectSettings()
	model.OpenAdd()
	model.Input(ProjectSettingsName).SetValue("Customer")
	model.Input(ProjectSettingsValue).SetValue("customer")
	model.Input(ProjectSettingsParent).SetValue("work")
	_, cmd := model.Update(projectSettingsSpecial('s', tea.ModCtrl))
	intent := saveIntent(t, cmd)
	if intent.Plan.Kind != domain.ProjectEditAdd || intent.Plan.After[3].Value != "work.customer" {
		t.Fatalf("plan=%#v", intent.Plan)
	}
	if len(model.Projects) != 3 {
		t.Fatalf("catalog changed before result: %#v", model.Projects)
	}
	model.ApplyMessage(ProjectSettingsSavedMsg{Plan: intent.Plan})
	if model.Editing || len(model.Projects) != 4 || model.Projects[3].Value != "work.customer" {
		t.Fatalf("after save model=%#v", model)
	}
}

func TestProjectSettingsLabelOnlyEditDoesNotPlanTaskMappings(t *testing.T) {
	model := testProjectSettings()
	model.OpenEdit(0)
	model.Input(ProjectSettingsName).SetValue("Delivery")
	_, cmd := model.Update(projectSettingsSpecial('s', tea.ModCtrl))
	intent := saveIntent(t, cmd)
	if intent.Plan.Kind != domain.ProjectEditLabelOnly || intent.Plan.ValueChanged() || len(intent.Plan.PendingTaskMappings([]domain.Task{{UUID: "task", Status: "pending", Project: "work"}})) != 0 {
		t.Fatalf("plan=%#v", intent.Plan)
	}
}

func TestProjectSettingsRejectsInvalidHierarchyAndKeepsDraft(t *testing.T) {
	model := testProjectSettings()
	model.OpenEdit(0)
	model.Input(ProjectSettingsParent).SetValue("work.client")
	_, cmd := model.Update(projectSettingsSpecial('s', tea.ModCtrl))
	if cmd == nil {
		t.Fatal("expected validation command")
	}
	message, ok := cmd().(ProjectSettingsErrorMsg)
	if !ok || message.Err == nil {
		t.Fatalf("message=%#v", message)
	}
	model.ApplyMessage(message)
	if !model.Editing || model.Err == nil || model.Input(ProjectSettingsParent).Value() != "work.client" {
		t.Fatalf("draft was discarded: %#v", model)
	}
}

func TestProjectSettingsRemoveAsksAboutConfiguredChildren(t *testing.T) {
	model := testProjectSettings()
	model.Update(projectSettingsKey("d"))
	if !model.RemovePromptOpen || model.RemoveChildCount != 1 {
		t.Fatalf("prompt=%#v", model)
	}
	model.Update(projectSettingsKey("esc"))
	if model.RemovePromptOpen || len(model.Projects) != 3 {
		t.Fatal("cancel changed removal state")
	}

	model.Update(projectSettingsKey("d"))
	_, cmd := model.Update(projectSettingsKey("n"))
	no := saveIntent(t, cmd)
	if no.Plan.RemoveChildren || !reflect.DeepEqual(no.Plan.After.Values(), []string{"work.client", "personal"}) {
		t.Fatalf("no plan=%#v", no.Plan)
	}
	if len(model.Projects) != 3 {
		t.Fatal("removal applied before save result")
	}
	model.ApplyMessage(ProjectSettingsSavedMsg{Plan: no.Plan})

	model.OpenProjects(domain.ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
		{Name: "Deep", Value: "work.client.deep"},
		{Name: "Personal", Value: "personal"},
	})
	model.Update(projectSettingsKey("d"))
	_, cmd = model.Update(projectSettingsKey("y"))
	yes := saveIntent(t, cmd)
	if !yes.Plan.RemoveChildren || !reflect.DeepEqual(yes.Plan.After.Values(), []string{"personal"}) {
		t.Fatalf("yes plan=%#v", yes.Plan)
	}
}

func TestProjectSettingsRemoveWithoutChildrenEmitsSaveIntent(t *testing.T) {
	model := testProjectSettings()
	model.Selected = 2
	_, cmd := model.Update(projectSettingsKey("d"))
	intent := saveIntent(t, cmd)
	if intent.Plan.Kind != domain.ProjectEditRemove || intent.Plan.SourceValue != "personal" || intent.Plan.RemoveChildren {
		t.Fatalf("plan=%#v", intent.Plan)
	}
}

func TestProjectSettingsDirtyCancelRequiresExplicitDiscard(t *testing.T) {
	model := testProjectSettings()
	model.OpenEdit(0)
	model.Input(ProjectSettingsName).SetValue("Changed")
	model.Update(projectSettingsKey("esc"))
	if !model.DiscardPromptOpen || !model.Editing {
		t.Fatalf("dirty cancel did not prompt: %#v", model)
	}
	model.Update(projectSettingsKey("esc"))
	if model.DiscardPromptOpen || !model.Editing || !model.Dirty() {
		t.Fatal("prompt cancel discarded draft")
	}
	model.Update(projectSettingsKey("esc"))
	_, cmd := model.Update(projectSettingsKey("d"))
	if cmd == nil {
		t.Fatal("discard did not emit intent")
	}
	if _, ok := cmd().(ProjectSettingsDiscardMsg); !ok {
		t.Fatalf("message=%T", cmd())
	}
	if model.Editing || model.Projects[0].Name != "Work" {
		t.Fatalf("discard result=%#v", model)
	}
}

func TestProjectSettingsSaveFailureRetainsDraft(t *testing.T) {
	model := testProjectSettings()
	model.OpenEdit(0)
	model.Input(ProjectSettingsName).SetValue("Changed")
	_, cmd := model.Update(projectSettingsSpecial('s', tea.ModCtrl))
	intent := saveIntent(t, cmd)
	err := errors.New("permission denied")
	model.ApplyMessage(ProjectSettingsSavedMsg{Plan: intent.Plan, Err: err})
	if !model.Editing || model.Err != err || model.Input(ProjectSettingsName).Value() != "Changed" {
		t.Fatalf("save failure lost draft: %#v", model)
	}
}

func TestProjectSettingsSupportsResponsiveWidths(t *testing.T) {
	model := testProjectSettings()
	for _, size := range [][2]int{{120, 30}, {79, 20}, {49, 12}, {28, 8}, {0, 0}} {
		model.SetSize(size[0], size[1])
		view := sanitizeComponentRender(model.View())
		if size[0] == 0 || size[1] == 0 {
			if view != "" {
				t.Fatalf("size=%v view=%q", size, view)
			}
			continue
		}
		if lipgloss.Width(view) > size[0] {
			t.Fatalf("size=%v width=%d", size, lipgloss.Width(view))
		}
	}
}

func TestProjectSettingsSetProjectsCopiesInput(t *testing.T) {
	model := NewProjectSettings(Styles{})
	projects := domain.ProjectCatalog{{Name: "Work", Value: "work"}}
	model.OpenProjects(projects)
	model.Projects[0].Name = "Changed"
	if projects[0].Name != "Work" {
		t.Fatal("settings model shares catalog storage")
	}
}
