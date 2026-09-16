package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

func (m *Model) beginProjectMigration(request ProjectMigrationRequest) tea.Cmd {
	if m.MigrationRunning || m.MutationRunning || m.ProjectSaveRunning || m.Sync.Phase == SyncInFlight || m.Mode == ModeLoading || m.Mode == ModeRefreshing {
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
	m.MigrationSyncBefore = m.Sync
	m.Sync.UndoAvailable = false
	m.Sync.UndoUntil = time.Time{}
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
			m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{})
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
	if result.ChangedCount() == 0 {
		m.MigrationOutcome = nil
		m.Sync = m.MigrationSyncBefore
		m.finishProjectMigration()
		m.Mode = ModeReady
		switch {
		case result.Stale:
			m.Status = "Project migration preview is stale; review it again"
		case result.PreflightErr != nil:
			m.Status = "Project migration preflight failed: " + conciseError(result.PreflightErr)
		case result.CatalogErr != nil:
			m.Status = "Project catalog save failed: " + conciseError(result.CatalogErr)
		case result.Canceled:
			m.Status = "Project migration canceled"
		default:
			m.Status = "Projects saved; no tasks changed"
		}
		return m.scheduleSync()
	}

	resultCopy := result
	m.MigrationOutcome = &resultCopy
	m.markMigrationTaskChanges(m.now())
	m.Status = fmt.Sprintf("Projects saved; %d task(s) changed", result.ChangedCount())
	if result.FailedCount() > 0 || result.AmbiguousCount() > 0 {
		m.Status += fmt.Sprintf("; %d task outcome(s) need review", result.FailedCount()+result.AmbiguousCount())
	}
	m.Mode = ModeRefreshing
	m.MigrationRefreshPending = true
	return LoadTasksCommand(m.ctx, m.Client, "migration")
}

func (m *Model) markMigrationTaskChanges(now time.Time) {
	m.Sync.Unsynced = true
	m.Sync.UndoAvailable = false
	m.Sync.UndoUntil = time.Time{}
	if !m.Sync.Enabled {
		m.Sync.Phase = SyncDisabled
		m.Sync.NextAt = time.Time{}
		return
	}
	m.Sync.Phase = SyncWaiting
	m.Sync.NextAt = now.Add(m.Sync.MutationDelay)
}

func (m *Model) finishProjectMigration() {
	m.MigrationRunning = false
	m.MigrationRefreshPending = false
	m.PendingMigration = nil
	if m.SyncDeferred && m.Sync.NextAt.IsZero() && m.Sync.Enabled {
		m.Sync.NextAt = m.now()
	}
	m.SyncDeferred = false
}

func cloneProjectMigrationRequest(request ProjectMigrationRequest) ProjectMigrationRequest {
	request.Plan.Before = append(domain.ProjectCatalog(nil), request.Plan.Before...)
	request.Plan.After = append(domain.ProjectCatalog(nil), request.Plan.After...)
	request.Plan.Mappings = append([]domain.ProjectValueMapping(nil), request.Plan.Mappings...)
	request.TaskMappings = append([]domain.ProjectTaskMapping(nil), request.TaskMappings...)
	return request
}
