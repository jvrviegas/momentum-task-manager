package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

// View renders the complete responsive composition and configures Bubble Tea's
// alternate screen and limited mouse tracking.
func (m *Model) View() tea.View {
	if m == nil {
		return tea.NewView("")
	}
	layout := ui.ChooseLayout(m.Width, m.Height)
	view := tea.NewView(m.render(layout))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *Model) render(layout ui.Layout) string {
	if !layout.Usable {
		return ui.MinimumSizeMessage(layout.Width, layout.Height)
	}
	base := m.renderBase(layout)
	switch m.Overlay {
	case OverlayQuickAdd:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.QuickAdd.View())
	case OverlaySearch:
		return fitLines(lipgloss.JoinVertical(lipgloss.Top, base, m.Search.View()), layout.Width, layout.Height)
	case OverlayEdit:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Editor.View())
	case OverlayDetails:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Details.View())
	case OverlayConfirm:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Confirm.View())
	case OverlayHelp:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Help.View())
	case OverlayQuit:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Quit.View())
	case OverlayMinimum:
		return ui.MinimumSizeMessage(layout.Width, layout.Height)
	default:
		return fitLines(base, layout.Width, layout.Height)
	}
}

func (m *Model) renderBase(layout ui.Layout) string {
	nav := m.navigationItems()
	mainWidth := layout.ContentWidth
	if mainWidth < 1 {
		mainWidth = layout.Width
	}
	parts := make([]string, 0, 5)
	parts = append(parts, m.Styles.Title.Render("Momentum"))
	if layout.ShowTabs {
		parts = append(parts, ui.RenderTabs(string(m.ActiveView), nav, mainWidth, m.Styles))
	}
	title := strings.Title(string(m.ActiveView))
	if title == "" {
		title = "Inbox"
	}
	if m.Search.Active {
		title += "  /" + m.Search.Query
	}
	if m.ActiveView == ViewSettings {
		parts = append(parts, m.Styles.Title.Render(title))
	} else {
		parts = append(parts, fmt.Sprintf("%s  %s", m.Styles.Title.Render(title), m.Styles.Muted.Render(fmt.Sprintf("%d tasks", len(m.tasksFor(m.ActiveView))))))
	}
	bodyHeight := layout.Height - len(parts) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	if m.ActiveView == ViewSettings {
		parts = append(parts, m.ProjectSettings.ViewAt(mainWidth, bodyHeight))
	} else {
		parts = append(parts, m.renderTaskBody(mainWidth, bodyHeight))
	}
	parts = append(parts, m.renderFooter(mainWidth))
	main := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if layout.ShowSidebar {
		sidebarHeight := layout.Height - 2
		if sidebarHeight < 1 {
			sidebarHeight = 1
		}
		sidebar := ui.RenderSidebar(string(m.ActiveView), nav, layout.SidebarWidth, sidebarHeight, m.Styles)
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", main)
	}
	return main
}

type renderedTaskLine struct {
	text string
	uuid string
}

func (m *Model) renderTaskBody(width, height int) string {
	tasks := m.tasksFor(m.ActiveView)
	if len(tasks) == 0 {
		if m.Mode == ModeLoading {
			return m.Styles.Muted.Render("Loading tasks…")
		}
		if m.Err != nil {
			return m.Styles.Overdue.Render(ui.Truncate("Unable to load tasks: "+m.Err.Error(), width))
		}
		return ui.RenderEmpty(string(normalizeView(m.ActiveView)), width, m.Styles)
	}
	lines := make([]renderedTaskLine, 0, len(tasks)+3)
	selectedUUID := m.Selected[normalizeView(m.ActiveView)]
	selectedLine := 0
	if normalizeView(m.ActiveView) == ViewToday {
		for _, section := range m.Views.Sections {
			sectionTasks := section.Tasks
			if m.Search.Active {
				sectionTasks = ui.FilterTasks(sectionTasks, m.Search.Query)
			}
			if len(sectionTasks) == 0 {
				continue
			}
			lines = append(lines, renderedTaskLine{text: m.Styles.Muted.Render("  " + string(section.Group))})
			for _, task := range sectionTasks {
				if task.UUID == selectedUUID {
					selectedLine = len(lines)
				}
				lines = append(lines, renderedTaskLine{
					text: ui.RenderTaskRow(task, ui.TaskRowOptions{Width: width, ShowMetadata: ui.ChooseLayout(width, 20).ShowRowMetadata, Selected: task.UUID == selectedUUID, Now: m.nowTime(), Styles: m.Styles, Icons: m.Icons}),
					uuid: task.UUID,
				})
			}
		}
	} else {
		for _, task := range tasks {
			if task.UUID == selectedUUID {
				selectedLine = len(lines)
			}
			lines = append(lines, renderedTaskLine{
				text: ui.RenderTaskRow(task, ui.TaskRowOptions{Width: width, ShowMetadata: ui.ChooseLayout(width, 20).ShowRowMetadata, Selected: task.UUID == selectedUUID, Now: m.nowTime(), Styles: m.Styles, Icons: m.Icons}),
				uuid: task.UUID,
			})
		}
	}
	if selectedLine >= len(lines) {
		selectedLine = len(lines) - 1
	}
	start, end := ui.VisibleTaskRange(len(lines), selectedLine, height)
	visible := make([]string, 0, end-start)
	for _, line := range lines[start:end] {
		visible = append(visible, line.text)
	}
	return strings.Join(visible, "\n")
}

func (m *Model) renderFooter(width int) string {
	parts := make([]string, 0, 3)
	if m.Status != "" {
		parts = append(parts, m.Status)
	}
	if m.TaskContext != "" {
		parts = append(parts, "context: "+m.TaskContext)
	}
	if m.Search.Active {
		parts = append(parts, ui.SearchSummary(len(m.tasksForUnfiltered()), len(m.tasksFor(m.ActiveView)), m.Search.Query))
	}
	parts = append(parts, m.SyncStatus(m.nowTime()))
	parts = append(parts, "? help · q quit")
	return m.Styles.Muted.Render(ui.Truncate(strings.Join(parts, "  ·  "), width))
}

func (m *Model) tasksForUnfiltered() []domain.Task {
	if normalizeView(m.ActiveView) == ViewToday {
		return m.Views.Today
	}
	return m.Views.Inbox
}

func fitLines(value string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for index := range lines {
		if lipgloss.Width(lines[index]) > width {
			lines[index] = ui.Truncate(lines[index], width)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) nowTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now()
	}
	return m.now()
}
