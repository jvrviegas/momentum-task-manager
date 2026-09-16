package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

func (m *Model) handleProjectSettingsSave(plan domain.ProjectCatalogPlan) tea.Cmd {
	if plan.ValueChanged() {
		subprojects, err := domain.PlanUpdateProject(plan.Before, plan.SourceValue, plan.Destination, true)
		if err != nil {
			m.ProjectSettings.ApplyMessage(ui.ProjectSettingsErrorMsg{Err: err})
			return nil
		}
		return m.beginProjectRenamePreview(plan, subprojects)
	}
	return m.beginProjectCatalogSave(plan)
}

func (m *Model) beginProjectRenamePreview(exactPlan, subprojectsPlan domain.ProjectCatalogPlan) tea.Cmd {
	if m.ActiveView != ViewSettings || m.ProjectSaveRunning || m.MigrationRunning || m.RenamePreviewLoading {
		return nil
	}
	preserveOptions := m.ProjectRename.Open
	if !preserveOptions {
		m.ProjectRename.Mode = ui.ProjectRenameCatalogOnly
		m.ProjectRename.IncludeSubprojects = false
	}
	m.RenameExactPlan = cloneAppProjectPlan(exactPlan)
	m.RenameSubprojectsPlan = cloneAppProjectPlan(subprojectsPlan)
	m.RenamePreviewID++
	id := m.RenamePreviewID
	m.RenamePreviewLoading = true
	m.Status = "Loading project rename preview…"
	var client ProjectMigrationClient
	if m.MigrationCoordinator != nil {
		client = m.MigrationCoordinator.Client
	}
	return ProjectRenamePreviewCommand(m.ctx, client, id, m.RenameExactPlan, m.RenameSubprojectsPlan)
}

func (m *Model) applyProjectRenamePreview(message ProjectRenamePreviewMsg) tea.Cmd {
	if !m.RenamePreviewLoading || message.ID != m.RenamePreviewID {
		return nil
	}
	m.RenamePreviewLoading = false
	if message.Err != nil {
		m.ProjectSettings.ApplyMessage(ui.ProjectSettingsErrorMsg{Err: message.Err})
		if m.ProjectRename.Open {
			m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{Err: message.Err})
		}
		m.Status = "Project preview failed: " + conciseError(message.Err)
		return nil
	}
	m.RenameExactPlan = cloneAppProjectPlan(message.Preview.ExactPlan)
	m.RenameSubprojectsPlan = cloneAppProjectPlan(message.Preview.SubprojectsPlan)
	mode := m.ProjectRename.Mode
	includeSubprojects := m.ProjectRename.IncludeSubprojects
	m.ProjectRename.SetSize(m.Width, m.Height)
	m.ProjectRename.OpenPreview(message.Preview)
	if mode != ui.ProjectRenameCatalogOnly || includeSubprojects {
		m.ProjectRename.Mode = mode
		m.ProjectRename.IncludeSubprojects = includeSubprojects
		m.ProjectRename.UpdatePreview(message.Preview)
	}
	m.Status = "Review project rename preview"
	return nil
}

func (m *Model) refreshProjectRenamePreview(message ui.ProjectRenameOptionMsg) tea.Cmd {
	if m.ProjectRename.PreviewValid || m.RenamePreviewLoading {
		return nil
	}
	return m.beginProjectRenamePreview(m.RenameExactPlan, m.RenameSubprojectsPlan)
}

func (m *Model) handleProjectRenameConfirm(message ui.ProjectRenameConfirmMsg) tea.Cmd {
	if message.Mode == ui.ProjectRenameCatalogOnly {
		return m.beginProjectCatalogSave(message.Plan)
	}
	request := ProjectMigrationRequest{
		Plan:         cloneAppProjectPlan(message.Plan),
		Snapshot:     m.ProjectSnapshot,
		Scope:        migrationScopeFromPreview(message.Preview),
		TaskMappings: append([]domain.ProjectTaskMapping(nil), message.TaskMappings...),
		MigrateTasks: true,
	}
	return m.beginProjectMigration(request)
}

func migrationScopeFromPreview(preview ui.ProjectRenamePreview) taskwarrior.ProjectMigrationScope {
	return taskwarrior.ProjectMigrationScope{
		ContextName: preview.ContextName,
		ReadFilter:  preview.ReadFilter,
	}
}

func cloneAppProjectPlan(plan domain.ProjectCatalogPlan) domain.ProjectCatalogPlan {
	plan.Before = append(domain.ProjectCatalog(nil), plan.Before...)
	plan.After = append(domain.ProjectCatalog(nil), plan.After...)
	plan.Mappings = append([]domain.ProjectValueMapping(nil), plan.Mappings...)
	return plan
}

func projectMigrationError(result ProjectMigrationResult) error {
	switch {
	case result.PreflightErr != nil:
		return result.PreflightErr
	case result.CatalogErr != nil:
		return result.CatalogErr
	case result.Canceled:
		return fmt.Errorf("project migration canceled")
	default:
		return nil
	}
}
