package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/ui"
)

// handleMouse implements only the deliberate click, tab, and wheel behaviors.
func (m *Model) handleMouse(message tea.MouseMsg) tea.Cmd {
	if m.Overlay != OverlayNone {
		return nil
	}
	mouse := message.Mouse()
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.MoveSelection(-1)
		return nil
	case tea.MouseWheelDown:
		m.MoveSelection(1)
		return nil
	case tea.MouseLeft:
		// Continue below.
	default:
		return nil
	}
	layout := ui.ChooseLayout(m.Width, m.Height)
	if !layout.Usable {
		return nil
	}
	nav := m.navigationItems()
	if layout.ShowSidebar && mouse.X >= 0 && mouse.X < layout.SidebarWidth && mouse.Y >= 0 && mouse.Y < len(nav) {
		m.SwitchView(ViewName(nav[mouse.Y].Key))
		return nil
	}
	if layout.ShowTabs && mouse.Y == 1 {
		if index := ui.TabIndexAt(nav, layout.Width, mouse.X); index >= 0 {
			m.SwitchView(ViewName(nav[index].Key))
		}
		return nil
	}
	if uuid := m.taskUUIDAt(mouse.Y, layout); uuid != "" {
		tasks := m.tasksFor(m.ActiveView)
		for index, task := range tasks {
			if task.UUID == uuid {
				view := normalizeView(m.ActiveView)
				m.Selections[view] = index
				m.Selected[view] = uuid
				break
			}
		}
	}
	return nil
}

func (m *Model) taskUUIDAt(y int, layout ui.Layout) string {
	if m.ActiveView == ViewSettings {
		return ""
	}
	rowTop := 2
	if layout.ShowTabs {
		rowTop++
	}
	lineIndex := y - rowTop
	if lineIndex < 0 {
		return ""
	}
	lines := make([]string, 0)
	uuids := make([]string, 0)
	if normalizeView(m.ActiveView) == ViewToday {
		for _, section := range m.Views.Sections {
			sectionTasks := section.Tasks
			if m.Search.Active {
				sectionTasks = ui.FilterTasks(sectionTasks, m.Search.Query)
			}
			if len(sectionTasks) == 0 {
				continue
			}
			lines = append(lines, "section")
			uuids = append(uuids, "")
			for _, task := range sectionTasks {
				lines = append(lines, "task")
				uuids = append(uuids, task.UUID)
			}
		}
	} else {
		for _, task := range m.tasksFor(m.ActiveView) {
			lines = append(lines, "task")
			uuids = append(uuids, task.UUID)
		}
	}
	if lineIndex >= len(lines) {
		return ""
	}
	return uuids[lineIndex]
}

// HandleKey is exposed for embedding applications that need the same routing
// contract as Momentum's root model.
func (m *Model) HandleKey(key string) tea.Cmd {
	return m.updateKey(tea.KeyPressMsg(tea.Key{Text: key, Code: firstRune(key)}))
}

func firstRune(value string) rune {
	runes := []rune(value)
	if len(runes) == 0 {
		return 0
	}
	return runes[0]
}
