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

func renameKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func renamePreviewPlans(t *testing.T) (ProjectRenamePreview, domain.ProjectTaskMapping) {
	t.Helper()
	catalog := domain.ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
	}
	exact, err := domain.PlanUpdateProject(catalog, "work", domain.Project{Name: "Delivery", Value: "delivery"}, false)
	if err != nil {
		t.Fatal(err)
	}
	subprojects, err := domain.PlanUpdateProject(catalog, "work", domain.Project{Name: "Delivery", Value: "delivery"}, true)
	if err != nil {
		t.Fatal(err)
	}
	child := domain.ProjectTaskMapping{UUID: "child", OldValue: "work.client", NewValue: "delivery.client"}
	return ProjectRenamePreview{
		ExactPlan:           exact,
		SubprojectsPlan:     subprojects,
		ExactTasks:          []domain.ProjectTaskMapping{{UUID: "root", OldValue: "work", NewValue: "delivery"}},
		SubprojectTasks:     []domain.ProjectTaskMapping{{UUID: "root", OldValue: "work", NewValue: "delivery"}, child},
		ContextName:         "work-context",
		HasActiveContext:    true,
		DestinationTaskOnly: true,
	}, child
}

func TestProjectRenameCatalogOnlyIsDefaultAndShowsPreviewContext(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(NewStyles(ResolveTheme("dark", true)))
	model.SetSize(80, 24)
	model.OpenPreview(preview)
	if model.Mode != ProjectRenameCatalogOnly || model.IncludeSubprojects || !model.PreviewValid {
		t.Fatalf("initial model=%#v", model)
	}
	view := model.View()
	for _, want := range []string{"Rename project", "work → delivery", "Catalog only", "Active context: work-context", "Catalog descendants: 0", "Task descendants: 0", "Historical tasks retain their values", "native u cannot reverse"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %s", want, view)
		}
	}
	if strings.Contains(view, "Catalog + pending tasks") && !strings.Contains(view, "Catalog only") {
		t.Fatal("rename mode was not rendered")
	}
}

func TestProjectRenameIncludeSubprojectsShowsSeparateCatalogAndTaskCounts(t *testing.T) {
	preview, child := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.SetSize(80, 24)
	model.OpenPreview(preview)
	model.Update(renameKey("s"))
	if model.PreviewValid || !model.IncludeSubprojects {
		t.Fatalf("scope change did not invalidate preview: %#v", model)
	}
	preview.ExactTasks = nil
	preview.SubprojectTasks = []domain.ProjectTaskMapping{{UUID: "root", OldValue: "work", NewValue: "delivery"}, child}
	model.UpdatePreview(preview)
	view := model.View()
	for _, want := range []string{"Include subprojects", "Catalog descendants: 1", "Task descendants: 1", "Pending tasks: 2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %s", want, view)
		}
	}
}

func TestProjectRenameModeChangeInvalidatesPreviewAndEmitsIntent(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.OpenPreview(preview)
	_, cmd := model.Update(renameKey("t"))
	if model.Mode != ProjectRenameCatalogAndPending || model.PreviewValid || cmd == nil {
		t.Fatalf("mode change model=%#v cmd=%v", model, cmd)
	}
	message, ok := cmd().(ProjectRenameOptionMsg)
	if !ok || message.Mode != ProjectRenameCatalogAndPending {
		t.Fatalf("message=%#v", message)
	}
	model.UpdatePreview(preview)
	if !model.PreviewValid || model.Mode != ProjectRenameCatalogAndPending {
		t.Fatalf("preview refresh model=%#v", model)
	}
}

func TestProjectRenameRequiresExplicitConfirmationAndEmitsOnlyTaskValues(t *testing.T) {
	preview, child := renamePreviewPlans(t)
	preview.DestinationTaskOnly = false
	model := NewProjectRename(Styles{})
	model.OpenPreview(preview)
	model.Mode = ProjectRenameCatalogAndPending
	model.UpdatePreview(preview)
	model.Update(renameKey("enter"))
	if !model.ConfirmOpen || model.MergeWarningOpen {
		t.Fatalf("confirmation state=%#v", model)
	}
	_, cmd := model.Update(renameKey("y"))
	if cmd == nil {
		t.Fatal("confirmation did not emit intent")
	}
	message, ok := cmd().(ProjectRenameConfirmMsg)
	if !ok || message.Mode != ProjectRenameCatalogAndPending || len(message.TaskMappings) != 1 || message.TaskMappings[0].NewValue != "delivery" || message.TaskMappings[0].NewValue == "Delivery" || message.TaskMappings[0].UUID == child.UUID {
		t.Fatalf("message=%#v", message)
	}
}

func TestProjectRenameTaskOnlyDestinationRequiresEffectiveMergeWarning(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.SetSize(80, 24)
	model.OpenPreview(preview)
	model.Mode = ProjectRenameCatalogAndPending
	model.UpdatePreview(preview)
	model.Update(renameKey("enter"))
	if !model.MergeWarningOpen || model.ConfirmOpen {
		t.Fatalf("merge warning state=%#v", model)
	}
	view := strings.ToLower(model.View())
	if !strings.Contains(view, "effective merge") || !strings.Contains(view, "not in the catalog") {
		t.Fatalf("warning missing: %s", model.View())
	}
	model.Update(renameKey("enter"))
	if !model.ConfirmOpen || model.MergeWarningOpen {
		t.Fatalf("warning did not lead to confirmation: %#v", model)
	}
}

func TestProjectRenameNameOnlyAndZeroMatchPreviewsRemainCatalogOnly(t *testing.T) {
	catalog := domain.ProjectCatalog{{Name: "Work", Value: "work"}}
	labelPlan, err := domain.PlanUpdateProject(catalog, "work", domain.Project{Name: "Delivery", Value: "work"}, false)
	if err != nil {
		t.Fatal(err)
	}
	model := NewProjectRename(Styles{})
	model.SetSize(70, 20)
	model.OpenPreview(ProjectRenamePreview{ExactPlan: labelPlan, SubprojectsPlan: labelPlan, HasActiveContext: false})
	if model.CanMigrateTasks() || strings.Contains(model.View(), "Catalog + pending tasks") || !strings.Contains(model.View(), "no task migration") {
		t.Fatalf("label-only preview=%s", model.View())
	}

	exact, err := domain.PlanUpdateProject(catalog, "work", domain.Project{Name: "Delivery", Value: "delivery"}, false)
	if err != nil {
		t.Fatal(err)
	}
	model.OpenPreview(ProjectRenamePreview{ExactPlan: exact, SubprojectsPlan: exact, HasActiveContext: false})
	model.Mode = ProjectRenameCatalogAndPending
	model.UpdatePreview(ProjectRenamePreview{ExactPlan: exact, SubprojectsPlan: exact, HasActiveContext: false})
	if !strings.Contains(model.View(), "No active context") || !strings.Contains(model.View(), "Pending tasks: 0") {
		t.Fatalf("zero-match preview=%s", model.View())
	}
}

func TestProjectRenameCancelStaleAndErrorKeepActionableState(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.SetSize(70, 20)
	model.OpenPreview(preview)
	model.ApplyMessage(ProjectRenameResultMsg{Stale: true, Err: errors.New("context changed")})
	if model.PreviewValid || model.Err == nil || !model.Open {
		t.Fatalf("stale state=%#v", model)
	}
	model.ApplyMessage(ProjectRenameResultMsg{Err: errors.New("save failed")})
	if model.PreviewValid || model.Err == nil || !strings.Contains(model.View(), "save failed") {
		t.Fatalf("error state=%#v view=%s", model, model.View())
	}
	_, cmd := model.Update(renameKey("esc"))
	if cmd == nil || model.Open {
		t.Fatalf("cancel state=%#v cmd=%v", model, cmd)
	}
	if _, ok := cmd().(ProjectRenameCancelMsg); !ok {
		t.Fatalf("message=%T", cmd())
	}
}

func TestProjectRenameWidthsStayBounded(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.OpenPreview(preview)
	for _, size := range [][2]int{{120, 30}, {80, 24}, {49, 12}, {28, 8}, {0, 0}} {
		model.SetSize(size[0], size[1])
		view := model.View()
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

func TestProjectRenameUpdatePreviewCopiesTaskMappings(t *testing.T) {
	preview, _ := renamePreviewPlans(t)
	model := NewProjectRename(Styles{})
	model.OpenPreview(preview)
	preview.ExactTasks[0].NewValue = "changed"
	if reflect.DeepEqual(model.Preview.ExactTasks, preview.ExactTasks) {
		t.Fatal("preview shares task mapping storage")
	}
}
