package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// TaskListOptions controls a rendering pass without holding UI state.
type TaskListOptions struct {
	Width        int
	Height       int
	Selected     int
	ShowMetadata bool
	HideProject  bool
	Density      RowDensity
	Now          time.Time
	Styles       Styles
	Icons        Icons
}

// TaskRowOptions controls one task block. Comfortable density becomes a
// two-line logical block when metadata is useful; compact density is one line
// with only the overdue marker. HideProject is used by Inbox, where the
// project group heading already communicates that value. Highlight emphasises
// search terms in the title.
type TaskRowOptions struct {
	Width        int
	ShowMetadata bool
	HideProject  bool
	Completed    bool
	Density      RowDensity
	Selected     bool
	Highlight    string
	Now          time.Time
	Styles       Styles
	Icons        Icons
}

// TaskBlock is the render geometry shared by list rendering and callers that
// need to map terminal cells back to task identity. Lines never contain a
// blank separator between adjacent tasks.
type TaskBlock struct {
	UUID   string
	Lines  []string
	Height int
}

// NewTaskBlock renders one task without doing any I/O.
func NewTaskBlock(task domain.Task, options TaskRowOptions) TaskBlock {
	lines := renderTaskLines(task, options)
	return TaskBlock{UUID: task.UUID, Lines: lines, Height: len(lines)}
}

// RenderTaskBlock is the explicit block-oriented form used by the app
// compositor. RenderTaskRow remains as a compatibility convenience.
func RenderTaskBlock(task domain.Task, options TaskRowOptions) TaskBlock {
	return NewTaskBlock(task, options)
}

// RenderTaskList renders a height-aware visible window of task blocks. The
// selected block is kept visible where possible; a block taller than the
// viewport is clipped only as a last resort.
func RenderTaskList(tasks []domain.Task, options TaskListOptions) string {
	if options.Width <= 0 || options.Height <= 0 || len(tasks) == 0 {
		return ""
	}
	selected := options.Selected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(tasks) {
		selected = len(tasks) - 1
	}
	blocks := make([]TaskBlock, 0, len(tasks))
	for index, task := range tasks {
		rowOptions := TaskRowOptions{
			Width:        options.Width,
			ShowMetadata: options.ShowMetadata,
			HideProject:  options.HideProject,
			Density:      options.Density,
			Selected:     index == selected,
			Now:          options.Now,
			Styles:       options.Styles,
			Icons:        options.Icons,
		}
		blocks = append(blocks, RenderTaskBlock(task, rowOptions))
	}
	start, end := VisibleTaskBlockRange(blocks, blocks[selected].UUID, options.Height)
	lines := make([]string, 0, options.Height)
	for _, block := range blocks[start:end] {
		for _, line := range block.Lines {
			if len(lines) == options.Height {
				break
			}
			lines = append(lines, line)
		}
		if len(lines) == options.Height {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// VisibleTaskRange retains the logical one-line helper used by older callers.
// New multi-line callers should use VisibleTaskBlockRange.
func VisibleTaskRange(count, selected, height int) (int, int) {
	if count <= 0 || height <= 0 {
		return 0, 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= count {
		selected = count - 1
	}
	if height > count {
		height = count
	}
	start := selected - height + 1
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > count {
		end = count
		start = end - height
	}
	return start, end
}

// VisibleTaskBlockRange selects complete neighboring blocks when they fit in
// height. The selected block is the anchor so a keyboard move cannot leave it
// below the viewport. If one block itself is too tall, the returned range has
// exactly that block and the caller may clip its lines.
func VisibleTaskBlockRange(blocks []TaskBlock, selectedUUID string, height int) (int, int) {
	if len(blocks) == 0 || height <= 0 {
		return 0, 0
	}
	selected := 0
	foundSelection := false
	for index, block := range blocks {
		if block.UUID != "" && block.UUID == selectedUUID {
			selected = index
			foundSelection = true
			break
		}
	}
	if !foundSelection {
		for index, block := range blocks {
			if block.UUID != "" {
				selected = index
				break
			}
		}
	}
	blockHeight := func(index int) int {
		value := blocks[index].Height
		if value <= 0 {
			value = len(blocks[index].Lines)
		}
		if value <= 0 {
			value = 1
		}
		return value
	}
	if blockHeight(selected) > height {
		return selected, selected + 1
	}

	start, end := selected, selected+1
	used := blockHeight(selected)
	// Prefer context above the selection. This matches terminal-list behavior
	// and means the selected block remains wholly visible at the last position.
	for start > 0 && used+blockHeight(start-1) <= height {
		start--
		used += blockHeight(start)
	}
	for end < len(blocks) && used+blockHeight(end) <= height {
		end++
		used += blockHeight(end - 1)
	}
	return start, end
}

// RenderTaskRow renders a task block as newline-separated terminal rows.
func RenderTaskRow(task domain.Task, options TaskRowOptions) string {
	return strings.Join(renderTaskLines(task, options), "\n")
}

func renderTaskLines(task domain.Task, options TaskRowOptions) []string {
	if options.Width <= 0 {
		return []string{""}
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	if options.Density == "" {
		if options.Width >= NarrowBreakpoint {
			options.Density = DensityComfortable
		} else {
			options.Density = DensityCompact
		}
	}
	icons := options.Icons.orUnicode()
	styles := options.Styles
	width := options.Width

	state, stateTone, stateBold := icons.Pending, ToneMuted, false
	switch {
	case !task.IsPending():
		state = icons.Completed
	case task.Start != nil:
		state, stateTone, stateBold = icons.Active, ToneCyan, true
	case options.Selected:
		stateTone = ToneText
	}
	bg := FillNone
	gutter := txt(" ")
	if options.Selected {
		bg = FillSelection
		gutter = sp(icons.Selection, ToneAccent)
	}
	stateSpan := sp(state, stateTone)
	stateSpan.Bold = stateBold
	prefix := []Span{gutter, txt(" "), stateSpan, txt(" ")}
	prefixWidth := spansWidth(prefix)
	titleTone, titleBold := ToneText, options.Selected
	if !task.IsPending() {
		titleTone = ToneMuted
	}
	title := func(cells int) []Span {
		return highlightTitle(Truncate(task.Description, max(1, cells)), options.Highlight, titleTone, titleBold)
	}
	overdue := task.IsPending() && ClassifyOverdue(task, options.Now)

	if options.Density == DensityCompact {
		var trailing []Span
		if overdue {
			trailing = []Span{txt(" "), sp(icons.Overdue, ToneRed).bold()}
		}
		line := append(append(prefix, title(width-prefixWidth-spansWidth(trailing))...), grow())
		return []string{styles.Line(width, bg, append(line, trailing...)...)}
	}

	var slot []Span
	slotWidth := 0
	if options.Completed && task.End != nil {
		slot = []Span{muted(completionTime(task.End.In(options.Now.Location()), options.Now))}
		slotWidth = spansWidth(slot)
	} else if label, tone, ok := prioritySlot(task.Priority, icons); ok {
		slotWidth = max(6, lipgloss.Width(label))
		slot = []Span{sp(label, tone), gap(slotWidth - lipgloss.Width(label))}
	}
	titleWidth := width - prefixWidth
	if slotWidth > 0 {
		titleWidth -= slotWidth + 2
	}
	primary := append(append(prefix, title(titleWidth)...), grow())
	primary = append(primary, slot...)
	lines := []string{styles.Line(width, bg, primary...)}

	if !options.ShowMetadata {
		return lines
	}
	parts := taskMetadata(task, options, icons)
	if len(parts) == 0 {
		return lines
	}
	separator := muted(" " + icons.Dot + " ")
	available := width - prefixWidth
	total := func() int {
		sum := 0
		for index, part := range parts {
			if index > 0 {
				sum += spansWidth([]Span{separator})
			}
			sum += spansWidth(part.spans)
		}
		return sum
	}
	for len(parts) > 1 && total() > available {
		lowest := 0
		for index, part := range parts {
			if part.priority < parts[lowest].priority {
				lowest = index
			}
		}
		parts = append(parts[:lowest], parts[lowest+1:]...)
	}
	cont := txt(" ")
	if options.Selected {
		cont = sp(icons.SelectionCont, ToneAccent)
	}
	metadata := []Span{cont, gap(prefixWidth - 1)}
	for index, part := range parts {
		if index > 0 {
			metadata = append(metadata, separator)
		}
		metadata = append(metadata, part.spans...)
	}
	metadata = append(metadata, grow())
	return append(lines, styles.Line(width, bg, metadata...))
}

// metadataPart is one piece of a row's second line. Lower priorities drop
// first as width shrinks; the overdue phrase is the last to go.
type metadataPart struct {
	spans    []Span
	priority int
}

func taskMetadata(task domain.Task, options TaskRowOptions, icons Icons) []metadataPart {
	parts := make([]metadataPart, 0, 4)
	if task.Start != nil && task.IsPending() {
		parts = append(parts, metadataPart{spans: []Span{sp("Active", ToneCyan)}, priority: 3})
	}
	if !options.HideProject && task.Project != "" {
		parts = append(parts, metadataPart{spans: []Span{sp("#"+task.Project, ToneCyan)}, priority: 2})
	}
	if options.Completed || !task.IsPending() {
		return parts
	}
	if task.Due != nil {
		date := "Due " + formatDate(task.Due.In(options.Now.Location()), options.Now)
		if ClassifyOverdue(task, options.Now) {
			parts = append(parts, metadataPart{spans: []Span{
				sp(icons.Overdue+" Overdue", ToneRed), muted(" " + icons.Dot + " "), sp(date, ToneRed),
			}, priority: 5})
		} else {
			parts = append(parts, metadataPart{spans: []Span{muted(date)}, priority: 3})
		}
	}
	if task.Scheduled != nil {
		date := "Scheduled " + formatDate(task.Scheduled.In(options.Now.Location()), options.Now)
		parts = append(parts, metadataPart{spans: []Span{muted(date)}, priority: 3})
	}
	if task.Estimate != nil {
		parts = append(parts, metadataPart{spans: []Span{muted("Estimate " + task.Estimate.String())}, priority: 1})
	}
	return parts
}

// highlightTitle marks case-insensitive occurrences of search terms in bold
// underlined Accent so matches read without color too.
func highlightTitle(title, query string, tone Tone, bold bool) []Span {
	base := Span{Text: title, Tone: tone, Bold: bold}
	_, text, _ := parseSearchQuery(query)
	terms := strings.Fields(text)
	if len(terms) == 0 || title == "" {
		return []Span{base}
	}
	lower := strings.ToLower(title)
	marks := make([]bool, len(lower))
	for _, term := range terms {
		for from := 0; ; {
			index := strings.Index(lower[from:], term)
			if index < 0 {
				break
			}
			for i := from + index; i < from+index+len(term); i++ {
				marks[i] = true
			}
			from += index + len(term)
		}
	}
	if len(lower) != len(title) {
		return []Span{base}
	}
	spans := make([]Span, 0, 4)
	start := 0
	for index := 1; index <= len(title); index++ {
		if index < len(title) && marks[index] == marks[start] {
			continue
		}
		part := base
		part.Text = title[start:index]
		if marks[start] {
			part.Tone, part.Bold, part.Underline = ToneAccent, true, true
		}
		spans = append(spans, part)
		start = index
	}
	return spans
}

// prioritySlot is the right-edge slot label: arrow plus a short word.
func prioritySlot(priority string, icons Icons) (string, Tone, bool) {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case "H", "HIGH":
		return icons.High + " High", ToneHigh, true
	case "M", "MEDIUM":
		return icons.Medium + " Med", ToneMedium, true
	case "L", "LOW":
		return icons.Low + " Low", ToneLow, true
	default:
		return "", ToneText, false
	}
}

// RenderSectionHeader renders “Title  count  ─────” at column 0. The rule
// shrinks first, then the title truncates; the count stays.
func RenderSectionHeader(title string, count int, tone Tone, width int, styles Styles, icons Icons) string {
	if width <= 0 {
		return ""
	}
	icons = icons.orUnicode()
	countText := fmt.Sprintf("%d", count)
	if lipgloss.Width(title)+4+lipgloss.Width(countText) <= width {
		return styles.Line(width, FillNone, sp(title, tone).bold(), gap(2), muted(countText), gap(2), rule(icons.Rule))
	}
	room := width - 2 - lipgloss.Width(countText)
	if room < 1 {
		return styles.Line(width, FillNone, sp(title, tone).bold())
	}
	return styles.Line(width, FillNone, sp(Truncate(title, room), tone).bold(), gap(2), muted(countText))
}

// RenderMoreLine reports how many tasks sit below the visible window.
func RenderMoreLine(hidden, width int, styles Styles, icons Icons) string {
	icons = icons.orUnicode()
	return styles.Line(width, FillNone, muted(fmt.Sprintf("    %s %d more", icons.More, hidden)))
}

// PriorityLabel turns Taskwarrior's compact priority code into readable copy.
// Unknown values are retained so presentation never hides exported state.
func PriorityLabel(priority string) string {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case "H", "HIGH":
		return "High"
	case "M", "MEDIUM":
		return "Medium"
	case "L", "LOW":
		return "Low"
	default:
		return strings.TrimSpace(priority)
	}
}

// ClassifyOverdue is row presentation logic only: overdue means a due date
// before the local calendar date, not a stale scheduled date.
func ClassifyOverdue(task domain.Task, now time.Time) bool {
	if task.Due == nil {
		return false
	}
	loc := now.Location()
	due := task.Due.In(loc)
	today := now.In(loc)
	return time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, loc).Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc))
}

func relevantDate(task domain.Task, now time.Time) string {
	if task.Due != nil {
		return formatDate(task.Due.In(now.Location()), now)
	}
	if task.Scheduled != nil {
		return formatDate(task.Scheduled.In(now.Location()), now)
	}
	return ""
}

// formatDate is the friendly row date: Today/Tomorrow/Yesterday or “Sep 28”,
// plus the time unless it is local midnight (a date-only Taskwarrior value).
func formatDate(value, now time.Time) string {
	loc := now.Location()
	value = value.In(loc)
	clock := ""
	if value.Hour() != 0 || value.Minute() != 0 {
		clock = " " + value.Format("15:04")
	}
	return relativeDay(value, now) + clock
}

func relativeDay(value, now time.Time) string {
	loc := now.Location()
	today := now.In(loc)
	value = value.In(loc)
	sameDay := func(day time.Time) bool { return value.Year() == day.Year() && value.YearDay() == day.YearDay() }
	switch {
	case sameDay(today):
		return "Today"
	case sameDay(today.AddDate(0, 0, 1)):
		return "Tomorrow"
	case sameDay(today.AddDate(0, 0, -1)):
		return "Yesterday"
	default:
		return value.Format("Jan 2")
	}
}

// completionTime fills the Completed slot: HH:MM today and yesterday, the
// date and time earlier.
func completionTime(value, now time.Time) string {
	switch relativeDay(value, now) {
	case "Today", "Yesterday":
		return value.In(now.Location()).Format("15:04")
	default:
		return value.In(now.Location()).Format("Jan 2 15:04")
	}
}

// RowSummary is a plain-text equivalent useful for tests and accessibility.
func RowSummary(task domain.Task, now time.Time) string {
	parts := []string{fmt.Sprintf("%s [%s]", task.Description, task.Status)}
	if task.Project != "" {
		parts = append(parts, "#"+task.Project)
	}
	if date := relevantDate(task, now); date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}
