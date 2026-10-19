package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/ui"
)

func (m *Model) beginProjectMigration(request ProjectMigrationRequest) tea.Cmd {
	if m.MigrationRunning || m.MutationRunning || m.ProjectSaveRunning || m.Mode == ModeLoading || m.Mode == ModeRefreshing {
		m.Status = "Project migration is waiting for other work to finish"
		return nil
	}
	m.MigrationID++
	id := m.MigrationID
	request = cloneProjectMigrationRequest(request)
	m.PendingMigration = &request
	m.MigrationRunning = true
	m.MigrationRefreshPending = false
	m.MigrationOutcome = nil
	// The server does not undo a migration, so the window closes; it opens
	// again if no task moves.
	m.MigrationUndoBefore = m.Undo
	m.Undo = m.Undo.Close()
	m.Mode = ModeMutating
	m.Status = "Migrating projects…"
	return ProjectMigrationCommand(m.ctx, m.MigrationCoordinator, id, request)
}

func (m *Model) applyProjectMigration(message ProjectMigrationMsg) tea.Cmd {
	if !m.MigrationRunning || message.ID != m.MigrationID || m.PendingMigration == nil {
		return nil
	}
	request := *m.PendingMigration
	result := message.Result
	if result.CatalogSaved {
		m.Config.Projects = append(domain.ProjectCatalog(nil), request.Plan.After...)
		if result.CatalogSnapshot.Path != "" || result.CatalogSnapshot.Revision.Exists {
			m.ProjectSnapshot = result.CatalogSnapshot
		}
		m.ProjectSettings.ApplyMessage(ui.ProjectSettingsSavedMsg{Plan: request.Plan, Projects: m.Config.Projects})
		if m.ProjectRename.Open {
			// A migration applied in one request that changed no task keeps the
			// preview open, so a fresh one can retry it (SP-15).
			m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{Stale: result.Stale, Err: result.TaskErr})
		}
		m.refreshProjectSuggestions()
	} else if m.ProjectRename.Open {
		err := projectMigrationError(result)
		if err == nil && result.Stale {
			err = ErrProjectMigrationStale
		}
		if err != nil {
			m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{Stale: result.Stale, Err: err})
		}
	}
	if result.Changed == 0 {
		m.MigrationOutcome = nil
		m.Undo = m.MigrationUndoBefore
		m.finishProjectMigration()
		m.Mode = ModeReady
		switch {
		case result.CatalogSaved && result.Stale:
			m.Status = "Projects saved; no tasks moved because the preview is stale; review it again"
		case result.Stale:
			m.Status = "Project migration preview is stale; review it again"
		case result.TaskErr != nil:
			m.Status = "Projects saved; no tasks moved: " + conciseError(result.TaskErr)
		case result.PreflightErr != nil:
			m.Status = "Project migration preflight failed: " + conciseError(result.PreflightErr)
		case result.CatalogErr != nil:
			m.Status = "Project catalog save failed: " + conciseError(result.CatalogErr)
		case result.Canceled:
			m.Status = "Project migration canceled"
		default:
			m.Status = "Projects saved; no tasks changed"
		}
		return m.scheduleUndoTick()
	}

	resultCopy := result
	m.MigrationOutcome = &resultCopy
	m.Status = fmt.Sprintf("Projects saved; %d task(s) changed", result.Changed)
	m.Mode = ModeRefreshing
	m.MigrationRefreshPending = true
	return LoadTasksCommand(m.ctx, m.Client, "migration")
}

func (m *Model) finishProjectMigration() {
	m.MigrationRunning = false
	m.MigrationRefreshPending = false
	m.PendingMigration = nil
}

func cloneProjectMigrationRequest(request ProjectMigrationRequest) ProjectMigrationRequest {
	request.Plan.Before = append(domain.ProjectCatalog(nil), request.Plan.Before...)
	request.Plan.After = append(domain.ProjectCatalog(nil), request.Plan.After...)
	request.Plan.Mappings = append([]domain.ProjectValueMapping(nil), request.Plan.Mappings...)
	request.TaskMappings = append([]domain.ProjectTaskMapping(nil), request.TaskMappings...)
	return request
}
