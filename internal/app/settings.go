package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

func normalizeTaskView(view ViewName) ViewName {
	if view == ViewToday {
		return ViewToday
	}
	return ViewInbox
}

func (m *Model) updateSettingsKey(message tea.KeyPressMsg) tea.Cmd {
	if message.String() == "q" || message.String() == "ctrl+c" {
		if m.ProjectSaveRunning {
			return nil
		}
		if m.ProjectSettings.Dirty() {
			m.QuitAfterProjectSettings = true
			m.ProjectSettings.PromptDiscard()
			return nil
		}
		m.leaveSettings()
		return m.requestQuit()
	}
	if m.ProjectSaveRunning {
		return nil
	}
	wasDiscardPrompt := m.ProjectSettings.DiscardPromptOpen
	_, cmd := m.ProjectSettings.Update(message)
	if m.QuitAfterProjectSettings && wasDiscardPrompt && !m.ProjectSettings.DiscardPromptOpen && cmd == nil {
		m.QuitAfterProjectSettings = false
	}
	return cmd
}

func (m *Model) leaveSettings() {
	if m.ProjectSaveRunning || m.ProjectSettings.Dirty() {
		m.Status = "Save or discard project changes before leaving Settings"
		return
	}
	view := normalizeTaskView(m.SettingsReturnView)
	m.ProjectSettings.Close()
	m.ActiveView = view
	m.Focus = FocusList
	m.restoreSelection(view, m.Selected[view])
}

func (m *Model) beginProjectCatalogSave(plan domain.ProjectCatalogPlan) tea.Cmd {
	if m.ActiveView != ViewSettings || m.ProjectSaveRunning {
		return nil
	}
	m.ProjectSaveID++
	id := m.ProjectSaveID
	m.ProjectSaveRunning = true
	m.Status = "Saving projects…"
	if !plan.Changed() {
		snapshot := m.ProjectSnapshot
		return func() tea.Msg {
			return ProjectCatalogSaveMsg{ID: id, Plan: plan, Snapshot: snapshot}
		}
	}
	return ProjectCatalogSaveCommand(m.ctx, m.ProjectStore, m.ProjectSnapshot, plan, id)
}

func (m *Model) applyProjectCatalogSave(message ProjectCatalogSaveMsg) tea.Cmd {
	if !m.ProjectSaveRunning || message.ID != m.ProjectSaveID {
		return nil
	}
	m.ProjectSaveRunning = false
	if message.Err != nil {
		m.ProjectSettings.ApplyMessage(ui.ProjectSettingsSavedMsg{Plan: message.Plan, Err: message.Err})
		m.Err = message.Err
		m.Status = "Project save failed: " + conciseError(message.Err)
		return nil
	}
	m.Err = nil
	m.Config.Projects = append(domain.ProjectCatalog(nil), message.Plan.After...)
	m.ProjectSettings.ApplyMessage(ui.ProjectSettingsSavedMsg{Plan: message.Plan, Projects: m.Config.Projects})
	if message.Snapshot.Path != "" || message.Snapshot.Revision.Exists {
		m.ProjectSnapshot = message.Snapshot
	}
	m.refreshProjectSuggestions()
	if m.QuitAfterProjectSettings {
		m.QuitAfterProjectSettings = false
		m.leaveSettings()
		return m.requestQuit()
	}
	if message.Plan.Changed() {
		m.Status = "Projects saved"
	} else {
		m.Status = "No project changes"
	}
	return nil
}

func (m *Model) refreshProjectSuggestions() {
	projects := m.Config.Projects.Merge(m.DiscoveredProjects)
	m.QuickAdd.SetProjectCatalog(projects)
	m.Editor.SetProjectCatalog(projects)
}

func (m *Model) navigationItems() []ui.NavItem {
	return []ui.NavItem{
		{Key: string(ViewInbox), Label: "Inbox", Count: len(m.Views.Inbox), Icon: m.Icons.Inbox},
		{Key: string(ViewToday), Label: "Today", Count: len(m.Views.Today), Icon: m.Icons.Today},
		{Key: string(ViewSettings), Label: "Settings", Icon: m.Icons.Settings, HideCount: true},
	}
}

func uniqueProjectValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
