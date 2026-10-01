package app

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
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
	if !layout.Usable || m.Overlay == OverlayMinimum {
		return ui.MinimumSizeMessage(layout.Width, layout.Height, m.Styles)
	}
	base := m.renderBase(layout)
	frame := ""
	switch m.Overlay {
	case OverlayQuickAdd:
		frame = m.QuickAdd.View()
	case OverlayEdit:
		frame = m.Editor.View()
	case OverlayDetails:
		frame = m.Details.View()
	case OverlayConfirm:
		frame = m.Confirm.View()
	case OverlayHelp:
		frame = m.Help.View()
	case OverlayQuit:
		frame = m.Quit.View()
	}
	if frame == "" {
		return base
	}
	lines := strings.Split(frame, "\n")
	at := ui.CenterPlacement(lipgloss.Width(lines[0]), len(lines), layout.Width, layout.Height)
	if m.Overlay == OverlayQuickAdd {
		// Quick capture sits high so the list stays visible below it.
		top := 5
		if layout.Narrow {
			top = 2
		}
		if layout.Height < 12 {
			top = 1
		}
		at.Y = max(0, min(top, layout.Height-len(lines)))
	}
	return m.Styles.Compose(base, lines, at, layout.Width, layout.Height)
}

// renderBase draws the shell row by row so every region keeps its exact
// cell geometry: wide has a rail, a │ divider, a 2-cell gutter, the main
// column and a right margin (plus the Wide+ pane); tabs modes have a tab row
// and its underline. The footer is a hints row and a Panel status bar.
func (m *Model) renderBase(layout ui.Layout) string {
	width, height := layout.Width, layout.Height
	searchOpen := m.Overlay == OverlaySearch && m.Search.Open
	geometry := layout.Geometry(searchOpen)
	mainWidth := layout.MainWidth
	nav := m.navigationItems()
	body := padLines(m.renderBody(mainWidth, geometry.BodyHeight), mainWidth, geometry.BodyHeight)
	search := ""
	if searchOpen {
		search = m.Search.ViewAt(mainWidth, fmt.Sprintf("%d matches", len(m.tasksFor(m.ActiveView))))
	}
	rows := make([]string, height)
	blank := strings.Repeat(" ", width)
	for index := range rows {
		rows[index] = blank
	}

	if layout.ShowSidebar {
		railHeight := height - 3
		sidebar := strings.Split(ui.RenderSidebar(string(m.ActiveView), nav, ui.SidebarWidth-1, railHeight, m.Styles, m.Icons), "\n")
		divider := m.Styles.Line(1, ui.FillNone, ui.Span{Text: m.Icons.Divider, Tone: ui.ToneBorder})
		var pane []string
		if layout.ShowPane {
			var selected *domain.Task
			if task, ok := m.SelectedTask(); ok && m.ActiveView != ViewSettings {
				selected = &task
			}
			pane = padLines(ui.RenderDetailsPane(selected, layout.PaneWidth-3, railHeight, m.nowTime(), m.Styles, m.Icons), layout.PaneWidth-3, railHeight)
		}
		header := m.headerLines(mainWidth)
		for y := 0; y < railHeight; y++ {
			main := strings.Repeat(" ", mainWidth)
			switch {
			case y < 2:
				main = header[y]
			case y == 2 && search != "":
				main = search
			case y >= geometry.BodyTop && y-geometry.BodyTop < len(body):
				main = body[y-geometry.BodyTop]
			}
			line := sidebar[y] + " " + divider + strings.Repeat(" ", ui.ContentGutter) + main + strings.Repeat(" ", ui.RightMargin)
			if layout.ShowPane {
				line += divider + " " + pane[y] + " "
			}
			rows[y] = line
		}
	} else {
		tabs := strings.Split(ui.RenderTabs(string(m.ActiveView), nav, width, m.Styles, m.Icons), "\n")
		copy(rows, tabs)
		left := strings.Repeat(" ", layout.MainLeft)
		right := strings.Repeat(" ", width-layout.MainLeft-mainWidth)
		if search != "" {
			rows[geometry.SearchTop] = left + search + right
		}
		for index, line := range body {
			rows[geometry.BodyTop+index] = left + line + right
		}
	}
	if geometry.HintsTop >= 0 {
		hints, extra := m.footerHints(layout)
		rows[geometry.HintsTop] = " " + ui.KeyHints(hints, extra, width-2, m.Styles) + " "
	}
	rows[geometry.StatusTop] = m.renderStatusBar(width, layout.Narrow)
	return strings.Join(rows, "\n")
}

// headerLines is the wide main-column header: bold title, Muted count and
// right-hand note, then a rule.
func (m *Model) headerLines(width int) []string {
	title, count, note := m.headerText()
	spans := []ui.Span{{Text: title, Bold: true}}
	if count != "" {
		spans = append(spans, ui.Span{Text: "  " + count, Tone: ui.ToneMuted})
	}
	spans = append(spans, ui.Span{Text: " ", Grow: true})
	if note != "" && ui.ChooseLayout(m.Width, m.Height).MainWidth >= 60 {
		spans = append(spans, ui.Span{Text: note, Tone: ui.ToneMuted})
	}
	return []string{
		m.Styles.Line(width, ui.FillNone, spans...),
		m.Styles.Line(width, ui.FillNone, ui.Span{Text: m.Icons.Rule, Tone: ui.ToneBorder, Grow: true}),
	}
}

func (m *Model) headerText() (title, count, note string) {
	title = viewLabel(m.ActiveView)
	tasks := len(m.tasksFor(m.ActiveView))
	switch m.ActiveView {
	case ViewSettings:
		return title, "", ""
	case ViewToday:
		note = "sorted by urgency"
	case ViewCompleted:
		note = "read-only"
	default:
		note = "grouped by project"
	}
	switch {
	case m.Mode == ModeLoading || (m.Err != nil && len(m.Tasks) == 0):
		count = ""
	case m.Search.Active:
		count = fmt.Sprintf("%d of %d", tasks, len(m.tasksForUnfiltered()))
	case m.ActiveView == ViewCompleted:
		count = fmt.Sprintf("%d in the last 30 days", tasks)
	default:
		count = plural(tasks, "task")
	}
	return title, count, note
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
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

func (m *Model) renderBody(width, height int) string {
	if m.ActiveView == ViewSettings {
		if m.ProjectRename.Open {
			return m.ProjectRename.ViewAt(width, height)
		}
		return m.ProjectSettings.ViewAt(width, height)
	}
	return m.renderTaskBody(width, height)
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
	// Row density follows the terminal tier, not the main-column width.
	narrow := width < ui.NarrowBreakpoint-2
	if m.Width > 0 {
		narrow = m.Width < ui.NarrowBreakpoint
	}
	density := ui.DensityComfortable
	if narrow {
		density = ui.DensityCompact
	}
	selectedUUID := m.Selected[normalizeView(m.ActiveView)]
	highlight := ""
	if m.Search.Active || (m.Search.Open && m.Overlay == OverlaySearch) {
		highlight = m.Search.Query
	}
	taskBlock := func(task domain.Task, hideProject bool) renderedTaskBlock {
		block := ui.RenderTaskBlock(task, ui.TaskRowOptions{
			Width:        width,
			ShowMetadata: !narrow,
			HideProject:  hideProject,
			Density:      density,
			Selected:     task.UUID == selectedUUID,
			Highlight:    highlight,
			Now:          m.nowTime(),
			Styles:       m.Styles,
			Icons:        m.Icons,
			Completed:    m.ActiveView == ViewCompleted,
		})
		return renderedTaskBlock{lines: block.Lines, uuid: task.UUID, selectable: true}
	}

	blocks := make([]renderedTaskBlock, 0, len(tasks)+6)
	section := func(title string, count int, tone ui.Tone) {
		if len(blocks) > 0 && !narrow {
			blocks = append(blocks, renderedTaskBlock{lines: []string{""}})
		}
		blocks = append(blocks, renderedTaskBlock{lines: []string{ui.RenderSectionHeader(title, count, tone, width, m.Styles, m.Icons)}})
	}
	filter := func(sectionTasks []domain.Task) []domain.Task {
		if m.Search.Active {
			return ui.FilterTasks(sectionTasks, m.Search.Query)
		}
		return sectionTasks
	}
	switch normalizeView(m.ActiveView) {
	case ViewToday:
		if len(m.Views.Sections) == 0 {
			for _, task := range tasks {
				blocks = append(blocks, taskBlock(task, false))
			}
			return blocks
		}
		for _, group := range m.Views.Sections {
			sectionTasks := filter(group.Tasks)
			if len(sectionTasks) == 0 {
				continue
			}
			tone := ui.ToneText
			if group.Group == domain.GroupOverdue {
				tone = ui.ToneRed
			}
			section(string(group.Group), len(sectionTasks), tone)
			for _, task := range sectionTasks {
				blocks = append(blocks, taskBlock(task, false))
			}
		}
	case ViewCompleted:
		for _, group := range m.Views.CompletedSections {
			sectionTasks := filter(group.Tasks)
			if len(sectionTasks) == 0 {
				continue
			}
			section(string(group.Group), len(sectionTasks), ui.ToneText)
			for _, task := range sectionTasks {
				blocks = append(blocks, taskBlock(task, false))
			}
		}
	default:
		for index, task := range tasks {
			if index == 0 || task.Project != tasks[index-1].Project {
				count := 0
				for _, other := range tasks[index:] {
					if other.Project != task.Project {
						break
					}
					count++
				}
				// Project headings carry the project identity in Inbox, so
				// the corresponding task metadata is deliberately omitted.
				if task.Project == "" {
					section("No project", count, ui.ToneMuted)
				} else {
					section("#"+task.Project, count, ui.ToneCyan)
				}
			}
			blocks = append(blocks, taskBlock(task, true))
		}
	}
	return blocks
}

// visibleTaskWindow is the block range shared by rendering and mouse hit
// testing. When tasks sit below the window, the last row reports them.
func (m *Model) visibleTaskWindow(blocks []renderedTaskBlock, height int) (start, end, hidden int) {
	selectedUUID := m.Selected[normalizeView(m.ActiveView)]
	window := func(budget int) (int, int, int) {
		start, end := visibleRenderedBlockRange(blocks, selectedUUID, budget)
		// Do not leave an unpaired section heading or spacing block at the
		// viewport edge when the next task is just below the fold.
		for end > start && !blocks[end-1].selectable {
			end--
		}
		hidden := 0
		for _, block := range blocks[end:] {
			if block.selectable {
				hidden++
			}
		}
		return start, end, hidden
	}
	start, end, hidden = window(height)
	if hidden > 0 && height > 1 {
		start, end, hidden = window(height - 1)
	}
	return start, end, hidden
}

func (m *Model) renderTaskBody(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	tasks := m.tasksFor(m.ActiveView)
	if len(tasks) == 0 {
		switch {
		case m.Mode == ModeLoading:
			return m.renderSkeleton(width, height)
		case m.Err != nil && len(m.Tasks) == 0:
			return m.renderLoadError("Could not load tasks", m.Err, width)
		case m.ActiveView == ViewCompleted && m.CompletedErr != nil:
			return m.renderLoadError("Could not load completed tasks", m.CompletedErr, width)
		case m.Search.Active:
			return m.Styles.Line(width, ui.FillNone, ui.Span{Text: fmt.Sprintf("  No tasks match %q", oneLine(m.Search.Query)), Tone: ui.ToneMuted})
		}
		return ui.RenderEmptyState(ui.EmptyOptions{
			View: string(normalizeView(m.ActiveView)), Width: width, Height: height,
			InboxCount: len(m.Views.Inbox), Styles: m.Styles, Icons: m.Icons,
		})
	}

	blocks := m.taskBlocks(width)
	start, end, hidden := m.visibleTaskWindow(blocks, height)
	lines := make([]string, 0, height)
	for _, block := range blocks[start:end] {
		for _, line := range block.lines {
			if len(lines) == height {
				break
			}
			lines = append(lines, line)
		}
	}
	if hidden > 0 && len(lines) < height {
		lines = append(lines, ui.RenderMoreLine(hidden, width, m.Styles, m.Icons))
	}
	return strings.Join(lines, "\n")
}

// renderSkeleton keeps the loaded layout so nothing jumps: ░ rows in Border
// at the real row positions.
func (m *Model) renderSkeleton(width, height int) string {
	pattern := [][2]int{{0, 9}, {4, 46}, {6, 28}, {4, 38}, {6, 22}, {0, 0}, {0, 11}, {4, 52}, {6, 31}, {4, 30}, {6, 24}}
	lines := make([]string, 0, height)
	for index := 0; index < height && index < len(pattern); index++ {
		indent, cells := pattern[index][0], pattern[index][1]
		if cells == 0 || indent >= width {
			lines = append(lines, "")
			continue
		}
		bar := ui.Span{Text: strings.Repeat(m.Icons.Skeleton, min(cells, width-indent)), Tone: ui.ToneBorder}
		spans := []ui.Span{{Text: strings.Repeat(" ", indent)}, bar}
		if indent == 0 {
			spans = append(spans, ui.Span{Text: "  "}, ui.Span{Text: strings.Repeat(m.Icons.Skeleton, 2), Tone: ui.ToneBorder})
		}
		lines = append(lines, m.Styles.Line(width, ui.FillNone, spans...))
	}
	return strings.Join(lines, "\n")
}

// renderLoadError states what failed, that nothing changed, and shows raw
// stderr in a Panel block so it can be copied as-is.
func (m *Model) renderLoadError(headline string, err error, width int) string {
	line := func(spans ...ui.Span) string { return m.Styles.Line(width, ui.FillNone, spans...) }
	lines := []string{"", line(ui.Span{Text: m.Icons.Error + " " + headline, Tone: ui.ToneRed, Bold: true})}
	summary := conciseError(err)
	stderr := ""
	var commandErr *taskwarrior.CommandError
	if errors.As(err, &commandErr) && strings.TrimSpace(commandErr.Stderr) != "" {
		summary = fmt.Sprintf("task %s failed", commandErr.Kind)
		if commandErr.ExitCode >= 0 {
			summary = fmt.Sprintf("task %s exited with status %d", commandErr.Kind, commandErr.ExitCode)
		}
		stderr = taskwarrior.Redact(strings.TrimSpace(commandErr.Stderr))
	}
	for _, text := range ui.WrapText(strings.TrimSuffix(summary, ".")+". Your data has not been changed.", width) {
		lines = append(lines, line(ui.Span{Text: text}))
	}
	if stderr != "" {
		lines = append(lines, "", m.Styles.Line(width, ui.FillPanel, ui.Span{Text: " stderr", Tone: ui.ToneMuted}))
		for _, text := range strings.Split(stderr, "\n") {
			for _, wrapped := range ui.WrapText(text, max(1, width-1)) {
				lines = append(lines, m.Styles.Line(width, ui.FillPanel, ui.Span{Text: " " + wrapped}))
			}
		}
	}
	lines = append(lines, "", line(ui.Span{Text: "Fix the cause, then press ", Tone: ui.ToneMuted}, ui.Span{Text: "r", Bold: true}, ui.Span{Text: " to retry.", Tone: ui.ToneMuted}))
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

// padLines splits a rendered block into exactly height lines of width cells.
func padLines(value string, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
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
	return lines
}

// footerHints are the hints row for the current state; extra sits on the
// right at Wide.
func (m *Model) footerHints(layout ui.Layout) (hints, extra []ui.Hint) {
	switch {
	case m.ActiveView == ViewSettings:
		hints = []ui.Hint{{Key: "esc", Label: "back"}, {Key: "1-3", Label: "views"}, {Key: "q", Label: "quit"}}
	case m.Overlay == OverlaySearch && m.Search.Open:
		hints = []ui.Hint{{Key: "enter", Label: "keep filter"}, {Key: "esc", Label: "clear"}}
	case m.Search.Active:
		hints = []ui.Hint{{Key: "esc", Label: "clear"}, {Key: "↑↓", Label: "move"}, {Key: "enter", Label: "details"}, {Key: "/", Label: "edit search"}}
	case m.Err != nil && len(m.Tasks) == 0:
		hints = []ui.Hint{{Key: "r", Label: "retry"}, {Key: "?", Label: "help"}, {Key: "q", Label: "quit"}}
	case m.ActiveView == ViewCompleted:
		hints = []ui.Hint{{Key: "/", Label: "search"}, {Key: "enter", Label: "details"}, {Key: "?", Label: "help"}, {Key: "q", Label: "quit"}}
	default:
		hints = []ui.Hint{{Key: "c", Label: "capture"}, {Key: "/", Label: "search"}, {Key: "enter", Label: "details"}, {Key: "?", Label: "help"}, {Key: "q", Label: "quit"}}
	}
	if layout.ShowSidebar && m.ActiveView != ViewSettings {
		extra = []ui.Hint{{Key: "1-4", Label: "views"}}
	}
	return hints, extra
}

// renderStatusBar is the full-width Panel row: message on the left, sync
// state on the right. Narrow shows hints when there is no message and only
// the sync glyph.
func (m *Model) renderStatusBar(width int, narrow bool) string {
	now := m.nowTime()
	left := m.statusMessage(now, narrow)
	glyph, word := m.syncState(now)
	right := append([]ui.Span{glyph}, word...)
	if narrow {
		right = []ui.Span{glyph}
		if len(left) == 0 {
			hints := []ui.Hint{{Key: "c", Label: "capture"}, {Key: "?", Label: "help"}}
			if m.ActiveView == ViewCompleted {
				hints = []ui.Hint{{Key: "/", Label: "search"}, {Key: "?", Label: "help"}}
			}
			for index, hint := range hints {
				if index > 0 {
					left = append(left, ui.Span{Text: "  "})
				}
				left = append(left, ui.Span{Text: hint.Key, Bold: true}, ui.Span{Text: " " + hint.Label, Tone: ui.ToneMuted})
			}
		}
	} else if m.TaskContext != "" {
		right = append([]ui.Span{{Text: "context ", Tone: ui.ToneMuted}, {Text: oneLine(m.TaskContext)}, {Text: "  "}}, right...)
	}
	spans := []ui.Span{{Text: " "}}
	room := width - 3 - spansWidth(right)
	spans = append(spans, clipSpans(left, room)...)
	spans = append(spans, ui.Span{Text: " ", Grow: true})
	spans = append(spans, right...)
	spans = append(spans, ui.Span{Text: " "})
	return m.Styles.Line(width, ui.FillPanel, spans...)
}

func (m *Model) statusMessage(now time.Time, narrow bool) []ui.Span {
	var spans []ui.Span
	switch {
	case m.Mode == ModeLoading:
		spans = []ui.Span{{Text: m.Icons.Syncing, Tone: ui.ToneAccent}, {Text: " Loading tasks from Taskwarrior…"}}
	case m.Err != nil:
		spans = []ui.Span{{Text: m.Icons.Error + " ", Tone: ui.ToneRed, Bold: true}, {Text: oneLine(conciseError(m.Err))}}
	case m.Search.Active:
		spans = []ui.Span{
			{Text: m.Icons.Search, Tone: ui.ToneAccent},
			{Text: " " + ui.SearchSummary(len(m.tasksForUnfiltered()), len(m.tasksFor(m.ActiveView)), m.Search.Query)},
			{Text: fmt.Sprintf(" %q", oneLine(m.Search.Query)), Tone: ui.ToneMuted},
		}
	case m.Status != "" && oneLine(m.Status) != m.SyncStatus(now):
		// The sync state already owns the right edge; do not repeat it.
		status := oneLine(m.Status)
		switch {
		case statusIsError(status):
			spans = []ui.Span{{Text: m.Icons.Error + " ", Tone: ui.ToneRed, Bold: true}, {Text: status}}
		case strings.HasPrefix(status, "Syncing"):
			spans = []ui.Span{{Text: m.Icons.Syncing + " ", Tone: ui.ToneAccent}, {Text: status}}
		default:
			spans = []ui.Span{{Text: m.Icons.Synced + " ", Tone: ui.ToneAccent}, {Text: status}}
		}
	}
	if m.Sync.UndoAvailable && !m.Sync.UndoUntil.IsZero() && m.Sync.MutationDelay > 0 {
		remaining := m.Sync.UndoUntil.Sub(now)
		if remaining > 0 {
			cells := 20
			if narrow {
				cells = 8
			}
			filled := int(math.Ceil(float64(cells) * remaining.Seconds() / m.Sync.MutationDelay.Seconds()))
			filled = min(cells, max(0, filled))
			if len(spans) > 0 {
				spans = append(spans, ui.Span{Text: "   "})
			}
			spans = append(spans,
				ui.Span{Text: " u ", Bold: true, Bg: ui.FillSelection}, ui.Span{Text: " undo  ", Tone: ui.ToneMuted},
				ui.Span{Text: strings.Repeat(m.Icons.RuleActive, filled), Tone: ui.ToneAccent},
				ui.Span{Text: strings.Repeat(m.Icons.Rule, cells-filled), Tone: ui.ToneBorder},
				ui.Span{Text: fmt.Sprintf(" %ds", int(math.Ceil(remaining.Seconds()))), Tone: ui.ToneMuted})
		}
	}
	return spans
}

func statusIsError(status string) bool {
	lower := strings.ToLower(status)
	for _, word := range []string{"failed", "unavailable", "unable", "could not", "error", "read-only", "deferred", "before leaving"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// syncState is a glyph and a word for every sync state, so it reads without
// color: ✓ ready, ↻ syncing, • local changes, ! failed, · local only.
func (m *Model) syncState(now time.Time) (ui.Span, []ui.Span) {
	text := m.SyncStatus(now)
	word := func(tone ui.Tone, bold bool, head, tail string) []ui.Span {
		spans := []ui.Span{{Text: " " + head, Tone: tone, Bold: bold}}
		if tail != "" {
			spans = append(spans, ui.Span{Text: " " + m.Icons.Dot + " " + tail, Tone: ui.ToneMuted})
		}
		return spans
	}
	split := func(value string) (string, string) {
		head, tail, _ := strings.Cut(value, " · ")
		return head, tail
	}
	switch {
	case m.Mode == ModeLoading:
		return ui.Span{Text: m.Icons.Syncing, Tone: ui.ToneMuted}, word(ui.ToneMuted, false, "Waiting for task export", "")
	case m.MigrationRunning:
		head, tail := split(text)
		return ui.Span{Text: m.Icons.Local, Tone: ui.ToneMedium}, word(ui.ToneMedium, false, head, tail)
	case !m.SyncConfigured || m.Sync.Phase == SyncDisabled:
		return ui.Span{Text: m.Icons.Off, Tone: ui.ToneMuted}, word(ui.ToneMuted, false, text, "")
	case m.Sync.UndoAvailable && !m.Sync.UndoUntil.IsZero():
		return ui.Span{Text: m.Icons.Local, Tone: ui.ToneMedium}, word(ui.ToneMedium, false, "Local changes", "not synced yet")
	case m.Sync.Phase == SyncRetrying:
		head, tail := split(text)
		return ui.Span{Text: m.Icons.Failed, Tone: ui.ToneRed, Bold: true}, word(ui.ToneRed, true, head, tail)
	case m.Sync.Phase == SyncInFlight:
		return ui.Span{Text: m.Icons.Syncing, Tone: ui.ToneAccent}, word(ui.ToneAccent, false, text, "")
	default:
		return ui.Span{Text: m.Icons.Synced, Tone: ui.ToneMuted}, word(ui.ToneMuted, false, text, "")
	}
}

func spansWidth(spans []ui.Span) int {
	total := 0
	for _, span := range spans {
		if !span.Grow {
			total += lipgloss.Width(span.Text)
		}
	}
	return total
}

// clipSpans keeps a message within width, ending in … when it is cut.
func clipSpans(spans []ui.Span, width int) []ui.Span {
	if width <= 0 {
		return nil
	}
	if spansWidth(spans) <= width {
		return spans
	}
	out := make([]ui.Span, 0, len(spans))
	budget := width - 1
	for _, span := range spans {
		w := lipgloss.Width(span.Text)
		if w <= budget {
			out = append(out, span)
			budget -= w
			continue
		}
		span.Text = ui.Truncate(span.Text, budget+1)
		out = append(out, span)
		return out
	}
	return append(out, ui.Span{Text: "…"})
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
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

func (m *Model) nowTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now()
	}
	return m.now()
}
