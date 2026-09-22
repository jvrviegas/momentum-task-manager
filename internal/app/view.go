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
		// Search is part of renderBase's vertical contract. It is not appended
		// after a fitted base, so opening it cannot clip the page header.
		return fitLines(base, layout.Width, layout.Height)
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
	case OverlayPlanner:
		return lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, m.Planner.View())
	case OverlayMinimum:
		return ui.MinimumSizeMessage(layout.Width, layout.Height)
	default:
		return fitLines(base, layout.Width, layout.Height)
	}
}

func (m *Model) renderBase(layout ui.Layout) string {
	mainWidth := layout.MainWidth
	if mainWidth < 1 {
		mainWidth = layout.ContentWidth
	}
	if mainWidth < 1 {
		mainWidth = layout.Width
	}
	geometry := layout.Geometry(m.Overlay == OverlaySearch && m.Search.Open)
	nav := m.navigationItems()

	parts := make([]string, 0, 8)
	header := m.Styles.PageTitle.Render(ui.Truncate(m.pageHeader(), mainWidth))
	parts = append(parts, ui.PadRight(header, mainWidth))
	if layout.ShowTabs {
		parts = append(parts, ui.PadRight(ui.RenderTabs(string(m.ActiveView), nav, mainWidth, m.Styles), mainWidth))
	}
	if geometry.ContentGap > 0 {
		parts = append(parts, strings.Repeat(" ", mainWidth))
	}
	if geometry.SearchHeight > 0 {
		parts = append(parts, ui.PadRight(ui.Truncate(m.Search.ViewAt(mainWidth), mainWidth), mainWidth))
	}

	bodyHeight := geometry.BodyHeight
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	var body string
	if m.ActiveView == ViewSettings {
		if m.ProjectRename.Open {
			body = m.ProjectRename.ViewAt(mainWidth, bodyHeight)
		} else {
			body = m.ProjectSettings.ViewAt(mainWidth, bodyHeight)
		}
	} else {
		body = m.renderTaskBody(mainWidth, bodyHeight)
	}
	parts = append(parts, padBlock(body, mainWidth, bodyHeight))
	if geometry.FooterGap > 0 {
		parts = append(parts, strings.Repeat(" ", mainWidth))
	}
	parts = append(parts, m.renderFooter(mainWidth, geometry.FooterHeight))

	main := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if layout.ShowSidebar {
		sidebar := ui.RenderSidebar(string(m.ActiveView), nav, layout.SidebarWidth, layout.Height, m.Styles)
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, strings.Repeat(" ", layout.ContentGutter), main)
	}
	return main
}

func (m *Model) pageHeader() string {
	label := viewLabel(m.ActiveView)
	count := ""
	if m.ActiveView != ViewSettings {
		count = fmt.Sprintf(" · %d tasks", len(m.tasksFor(m.ActiveView)))
	}
	query := ""
	if m.Search.Active {
		query = " · /" + oneLine(m.Search.Query)
	}
	return "Momentum  ·  " + label + count + query
}

func viewLabel(view ViewName) string {
	switch view {
	case ViewToday:
		return "Today"
	case ViewCompleted:
		return "Completed"
	case ViewSettings:
		return "Settings"
	default:
		return "Inbox"
	}
}

type renderedTaskBlock struct {
	lines      []string
	uuid       string
	selectable bool
}

func (b renderedTaskBlock) height() int {
	if len(b.lines) == 0 {
		return 1
	}
	return len(b.lines)
}

func (m *Model) taskBlocks(width int) []renderedTaskBlock {
	tasks := m.tasksFor(m.ActiveView)
	if len(tasks) == 0 {
		return nil
	}
	rowLayout := ui.ChooseLayout(width, ui.MinimumHeight)
	selectedUUID := m.Selected[normalizeView(m.ActiveView)]
	options := func(task domain.Task, selected bool, hideProject bool) renderedTaskBlock {
		block := ui.RenderTaskBlock(task, ui.TaskRowOptions{
			Width:           width,
			ShowMetadata:    rowLayout.ShowRowMetadata,
			HideProject:     hideProject,
			CompactMetadata: rowLayout.RowDensity == ui.DensityCompact,
			Density:         rowLayout.RowDensity,
			Selected:        selected,
			Now:             m.nowTime(),
			Styles:          m.Styles,
			Icons:           m.Icons,
			Completed:       m.ActiveView == ViewCompleted,
		})
		return renderedTaskBlock{lines: block.Lines, uuid: task.UUID, selectable: true}
	}

	blocks := make([]renderedTaskBlock, 0, len(tasks)+3)
	appendSectionGap := func() {
		if len(blocks) == 0 {
			return
		}
		gap := make([]string, ui.SectionGap)
		blocks = append(blocks, renderedTaskBlock{lines: gap})
	}
	view := normalizeView(m.ActiveView)
	if view == ViewToday {
		if len(m.Views.Sections) == 0 {
			for _, task := range tasks {
				blocks = append(blocks, options(task, task.UUID == selectedUUID, false))
			}
			return blocks
		}
		for _, section := range m.Views.Sections {
			sectionTasks := section.Tasks
			if m.Search.Active {
				sectionTasks = ui.FilterTasks(sectionTasks, m.Search.Query)
			}
			if len(sectionTasks) == 0 {
				continue
			}
			appendSectionGap()
			heading := fmt.Sprintf("%s · %d", section.Group, len(sectionTasks))
			blocks = append(blocks, renderedTaskBlock{
				lines: []string{m.Styles.SectionTitle.Render(ui.Truncate(heading, width))},
			})
			for _, task := range sectionTasks {
				blocks = append(blocks, options(task, task.UUID == selectedUUID, false))
			}
		}
		return blocks
	}
	if view == ViewCompleted {
		for _, section := range m.Views.CompletedSections {
			sectionTasks := section.Tasks
			if m.Search.Active {
				sectionTasks = ui.FilterTasks(sectionTasks, m.Search.Query)
			}
			if len(sectionTasks) == 0 {
				continue
			}
			appendSectionGap()
			heading := fmt.Sprintf("%s · %d", section.Group, len(sectionTasks))
			blocks = append(blocks, renderedTaskBlock{lines: []string{m.Styles.SectionTitle.Render(ui.Truncate(heading, width))}})
			for _, task := range sectionTasks {
				blocks = append(blocks, options(task, task.UUID == selectedUUID, false))
			}
		}
		return blocks
	}

	for index, task := range tasks {
		if index == 0 || task.Project != tasks[index-1].Project {
			appendSectionGap()
			project := task.Project
			if project == "" {
				project = "No project"
			}
			// Project headings carry the project identity in Inbox, so the
			// corresponding task metadata is deliberately omitted.
			blocks = append(blocks, renderedTaskBlock{
				lines: []string{m.Styles.SectionTitle.Render(ui.Truncate(project, width))},
			})
		}
		blocks = append(blocks, options(task, task.UUID == selectedUUID, true))
	}
	return blocks
}

func (m *Model) renderTaskBody(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	tasks := m.tasksFor(m.ActiveView)
	if len(tasks) == 0 {
		if m.Mode == ModeLoading {
			return m.Styles.Muted.Render("Loading tasks…")
		}
		if m.Err != nil {
			return m.Styles.Error.Render(ui.Truncate("Unable to load tasks: "+oneLine(m.Err.Error()), width))
		}
		if m.ActiveView == ViewCompleted && m.CompletedErr != nil {
			return m.Styles.Error.Render(ui.Truncate("Unable to load completed tasks: "+oneLine(m.CompletedErr.Error()), width))
		}
		return ui.RenderEmpty(string(normalizeView(m.ActiveView)), width, m.Styles)
	}

	blocks := m.taskBlocks(width)
	selectedUUID := m.Selected[normalizeView(m.ActiveView)]
	start, end := visibleRenderedBlockRange(blocks, selectedUUID, height)
	// Do not leave an unpaired section heading or spacing block at the
	// viewport edge when the next task is just below the fold.
	for end > start && !blocks[end-1].selectable {
		end--
	}
	lines := make([]string, 0, height)
	for _, block := range blocks[start:end] {
		for _, line := range block.lines {
			if len(lines) == height {
				break
			}
			lines = append(lines, line)
		}
		if len(lines) == height {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func visibleRenderedBlockRange(blocks []renderedTaskBlock, selectedUUID string, height int) (int, int) {
	if len(blocks) == 0 || height <= 0 {
		return 0, 0
	}
	tasks := make([]ui.TaskBlock, len(blocks))
	for index, block := range blocks {
		tasks[index] = ui.TaskBlock{UUID: block.uuid, Height: block.height(), Lines: block.lines}
	}
	start, end := ui.VisibleTaskBlockRange(tasks, selectedUUID, height)
	if start == 0 || start >= end || blocks[start].selectable {
		return start, end
	}

	// When the selected row is just after a section gap, include the heading
	// context if the current viewport has room. This is still a block budget,
	// and never sacrifices the selected task to show older context.
	contextStart := start
	for contextStart > 0 && !blocks[contextStart-1].selectable {
		contextStart--
	}
	contextHeight := 0
	for index := contextStart; index < start; index++ {
		contextHeight += blocks[index].height()
	}
	if contextHeight+blockRangeHeight(blocks, start, end) <= height {
		start = contextStart
	}
	return start, end
}

func blockRangeHeight(blocks []renderedTaskBlock, start, end int) int {
	height := 0
	for index := start; index < end && index < len(blocks); index++ {
		height += blocks[index].height()
	}
	return height
}

func padBlock(value string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for index := range lines {
		lines[index] = ui.PadRight(ui.Truncate(lines[index], width), width)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderFooter(width int, requestedHeight ...int) string {
	height := 1
	if len(requestedHeight) > 0 {
		height = requestedHeight[0]
	}
	if width <= 0 || height <= 0 {
		return ""
	}
	statusParts := make([]string, 0, 5)
	if m.Err != nil {
		statusParts = append(statusParts, "Error: "+oneLine(conciseError(m.Err)))
	} else if m.Status != "" {
		statusParts = append(statusParts, oneLine(m.Status))
	}
	if m.Search.Active {
		statusParts = append(statusParts, ui.SearchSummary(len(m.tasksForUnfiltered()), len(m.tasksFor(m.ActiveView)), m.Search.Query))
	}
	if m.TaskContext != "" {
		statusParts = append(statusParts, "Context: "+oneLine(m.TaskContext))
	}
	statusParts = append(statusParts, m.SyncStatus(m.nowTime()))
	status := footerStatus(statusParts, width)
	actions := "[c] Create  [/] Search  [?] Help  [q] Quit"
	if height == 1 {
		separator := "  ·  "
		available := width - lipgloss.Width(status) - lipgloss.Width(separator)
		if available > 0 {
			return ui.PadRight(ui.Truncate(status+separator+ui.Truncate(actions, available), width), width)
		}
		return ui.PadRight(ui.Truncate(status, width), width)
	}
	actionLine := m.Styles.KeyHint.Render(ui.Truncate(actions, width))
	statusLine := m.Styles.Status.Render(status)
	if m.Err != nil {
		statusLine = m.Styles.Error.Render(status)
	}
	lines := []string{
		ui.PadRight(actionLine, width),
		ui.PadRight(statusLine, width),
	}
	return strings.Join(lines, "\n")
}

func footerStatus(parts []string, width int) string {
	parts = nonEmpty(parts)
	if len(parts) == 0 || width <= 0 {
		return ""
	}
	if len(parts) == 1 {
		return ui.Truncate(parts[0], width)
	}
	separator := "  ·  "
	sync := ui.Truncate(parts[len(parts)-1], width)
	if lipgloss.Width(sync) >= width {
		return sync
	}
	prefixWidth := width - lipgloss.Width(sync) - lipgloss.Width(separator)
	if prefixWidth <= 0 {
		return sync
	}
	prefix := ui.Truncate(strings.Join(parts[:len(parts)-1], separator), prefixWidth)
	if prefix == "" {
		return sync
	}
	return prefix + separator + sync
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func nonEmpty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func (m *Model) tasksForUnfiltered() []domain.Task {
	switch normalizeView(m.ActiveView) {
	case ViewToday:
		return m.Views.Today
	case ViewCompleted:
		return m.Views.Completed
	default:
		return m.Views.Inbox
	}
}

func fitLines(value string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		// Keep the page header and the final status line if an unfamiliar
		// component exceeds its budget. Normal shell components are already
		// height-aware, so this is a final safety net rather than scrolling.
		if height == 1 {
			lines = lines[:1]
		} else {
			lines = append([]string{lines[0]}, lines[len(lines)-(height-1):]...)
		}
	}
	for index := range lines {
		lines[index] = ui.Truncate(lines[index], width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) nowTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now()
	}
	return m.now()
}
