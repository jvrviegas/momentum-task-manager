package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/ui"
)

// handleMouse implements only the deliberate click, tab, and wheel behaviors.
// Hit regions are derived from the same layout and task blocks used by View.
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
	if layout.ShowSidebar && mouse.X >= 0 && mouse.X < ui.SidebarWidth {
		if index := ui.SidebarItemAt(nav, layout.Height-3, mouse.Y); index >= 0 {
			m.SwitchView(ViewName(nav[index].Key))
		}
		return nil
	}
	if layout.ShowTabs && mouse.Y == 0 {
		if index := ui.TabIndexAt(string(m.ActiveView), nav, layout.Width, mouse.X, m.Icons); index >= 0 {
			m.SwitchView(ViewName(nav[index].Key))
		}
		return nil
	}
	if mouse.X < layout.MainLeft || mouse.X >= layout.MainLeft+layout.MainWidth {
		// The divider, gutter, margins and pane are not task hit targets.
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
	if m.ActiveView == ViewSettings || !layout.Usable {
		return ""
	}
	geometry := layout.Geometry(m.Overlay == OverlaySearch && m.Search.Open)
	lineIndex := y - geometry.BodyTop
	if lineIndex < 0 || lineIndex >= geometry.BodyHeight {
		return ""
	}
	blocks := m.taskBlocks(layout.MainWidth)
	if len(blocks) == 0 {
		return ""
	}
	start, end, _ := m.visibleTaskWindow(blocks, geometry.BodyHeight)
	for _, block := range blocks[start:end] {
		for range block.lines {
			if lineIndex == 0 && block.selectable {
				return block.uuid
			}
			lineIndex--
			if lineIndex < 0 {
				return ""
			}
		}
	}
	return ""
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
