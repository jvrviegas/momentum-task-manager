package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

// SettingsSection is the Settings screen tab switches between.
type SettingsSection string

const (
	SettingsProjects   SettingsSection = ""
	SettingsAppearance SettingsSection = "appearance"
)

const themeLeaveStatus = "Save (enter) or discard (esc) the theme change first"

func normalizeTaskView(view ViewName) ViewName {
	switch view {
	case ViewToday, ViewCompleted:
		return view
	default:
		return ViewInbox
	}
}

func (m *Model) updateSettingsKey(message tea.KeyPressMsg) tea.Cmd {
	if m.ProjectRename.Open {
		if m.RenamePreviewLoading || m.MigrationRunning || m.ProjectSaveRunning {
			return nil
		}
		_, cmd := m.ProjectRename.Update(message)
		return cmd
	}
	if m.SettingsSection == SettingsAppearance {
		return m.updateAppearanceKey(message)
	}
	if !m.ProjectSettings.Editing && !m.ProjectSettings.RemovePromptOpen && !m.ProjectSettings.DiscardPromptOpen {
		switch message.String() {
		case "1":
			m.SwitchView(ViewInbox)
			return nil
		case "2":
			m.SwitchView(ViewToday)
			return nil
		case "3":
			m.SwitchView(ViewCompleted)
			return nil
		case "tab":
			if !m.ProjectSaveRunning && !m.MigrationRunning {
				m.SettingsSection = SettingsAppearance
			}
			return nil
		}
	}
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
	if m.Appearance.Saving || m.Appearance.Dirty() {
		m.Status = themeLeaveStatus
		return
	}
	view := normalizeTaskView(m.SettingsReturnView)
	m.ProjectSettings.Close()
	m.SettingsSection = SettingsProjects
	m.ActiveView = view
	m.Focus = FocusList
	m.restoreSelection(view, m.Selected[view])
}

// updateAppearanceKey routes keys in Settings → Appearance and repaints the
// app with the draft theme so every change previews live.
func (m *Model) updateAppearanceKey(message tea.KeyPressMsg) tea.Cmd {
	pending := m.Appearance.Saving || m.Appearance.Dirty()
	switch message.String() {
	case "1":
		m.SwitchView(ViewInbox)
		return nil
	case "2":
		m.SwitchView(ViewToday)
		return nil
	case "3":
		m.SwitchView(ViewCompleted)
		return nil
	case "tab":
		if pending {
			m.Status = themeLeaveStatus
		} else {
			m.SettingsSection = SettingsProjects
		}
		return nil
	case "q", "ctrl+c":
		if pending {
			m.Status = themeLeaveStatus
			return nil
		}
		m.leaveSettings()
		return m.requestQuit()
	}
	cmd := m.Appearance.Update(message)
	m.applyTheme(m.Appearance.Draft)
	return cmd
}

// applyTheme repaints every component with the palette for mode.
func (m *Model) applyTheme(mode string) {
	styles := ui.NewStyles(ui.ResolveTheme(mode, m.DarkBackground))
	if styles == m.Styles {
		return
	}
	m.Styles = styles
	m.QuickAdd.Styles = styles
	m.Search.Styles = styles
	m.Editor.Styles = styles
	m.Details.Styles = styles
	m.Confirm.Styles = styles
	m.Help.Styles = styles
	m.Quit.Styles = styles
	m.ProjectSettings.Styles = styles
	m.ProjectRename.Styles = styles
	m.Appearance.Styles = styles
	m.Planner.Styles = styles
}

func (m *Model) beginThemeSave(theme string) tea.Cmd {
	m.Status = "Saving theme…"
	return ThemeSaveCommand(m.ctx, m.ProjectStore, m.ProjectSnapshot, theme)
}

func (m *Model) applyThemeSave(message ThemeSaveMsg) {
	if !m.Appearance.Saving {
		return
	}
	m.Appearance.ApplySaved(message.Err)
	if message.Err != nil {
		m.Status = "Theme save failed: " + conciseError(message.Err)
		return
	}
	m.Config.Theme = message.Theme
	if message.Snapshot.Path != "" || message.Snapshot.Revision.Exists {
		m.ProjectSnapshot = message.Snapshot
	}
	m.Status = "Theme saved"
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
		if m.ProjectRename.Open {
			m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{Err: message.Err})
		}
		m.Err = message.Err
		m.Status = "Project save failed: " + conciseError(message.Err)
		return nil
	}
	m.Err = nil
	m.Config.Projects = append(domain.ProjectCatalog(nil), message.Plan.After...)
	m.ProjectSettings.ApplyMessage(ui.ProjectSettingsSavedMsg{Plan: message.Plan, Projects: m.Config.Projects})
	if m.ProjectRename.Open {
		m.ProjectRename.ApplyMessage(ui.ProjectRenameResultMsg{})
	}
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

// navigationItems feeds the rail and tabs. Counts are omitted until known
// rather than shown as 0; Completed shows its count in the header instead.
func (m *Model) navigationItems() []ui.NavItem {
	unknown := m.Mode == ModeLoading || (m.Err != nil && len(m.Tasks) == 0)
	return []ui.NavItem{
		{Key: string(ViewInbox), Label: "Inbox", Count: len(m.Views.Inbox), Icon: m.Icons.Inbox, HideCount: unknown, Group: "Views"},
		{Key: string(ViewToday), Label: "Today", Count: len(m.Views.Today), Icon: m.Icons.Today, HideCount: unknown, Group: "Views"},
		{Key: string(ViewCompleted), Label: "Completed", Icon: m.Icons.CompletedView, HideCount: true, Group: "Views"},
		{Key: string(ViewSettings), Label: "Settings", Icon: m.Icons.Settings, HideCount: true, Group: "Manage"},
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
