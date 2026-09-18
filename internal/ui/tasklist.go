package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// TaskListOptions controls a rendering pass without holding UI state.
type TaskListOptions struct {
	Width           int
	Height          int
	Selected        int
	ShowMetadata    bool
	HideProject     bool
	CompactMetadata bool
	Density         RowDensity
	Now             time.Time
	Styles          Styles
	Icons           Icons
}

// TaskRowOptions controls one task block. Comfortable density becomes a
// two-line logical block when metadata is useful; compact density keeps only
// explicitly enabled short metadata. HideProject is used by Inbox, where the
// project group heading already communicates that value.
type TaskRowOptions struct {
	Width           int
	ShowMetadata    bool
	HideProject     bool
	CompactMetadata bool
	Density         RowDensity
	Selected        bool
	Now             time.Time
	Styles          Styles
	Icons           Icons
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
			Width:           options.Width,
			ShowMetadata:    options.ShowMetadata,
			HideProject:     options.HideProject,
			CompactMetadata: options.CompactMetadata,
			Density:         options.Density,
			Selected:        index == selected,
			Now:             options.Now,
			Styles:          options.Styles,
			Icons:           options.Icons,
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

	icon := options.Icons.Pending
	if task.Start != nil {
		icon = options.Icons.Active
	} else if !task.IsPending() {
		icon = options.Icons.Completed
	}

	gutter := ""
	if options.Selected {
		gutter = options.Styles.FocusGutter.Render(selectionMarker(options.Icons)) + " "
	}
	prefix := gutter + icon + " "
	var metadata []string
	if options.Density == DensityCompact {
		if options.CompactMetadata || options.ShowMetadata {
			metadata = compactMetadata(task, options)
		}
	} else if options.ShowMetadata {
		metadata = taskMetadata(task, options)
	}

	metadataSuffix := ""
	if options.Density == DensityCompact && len(metadata) > 0 {
		metadataSuffix = "  ·  " + strings.Join(metadata, "  ·  ")
	}
	descriptionWidth := options.Width - lipgloss.Width(prefix) - lipgloss.Width(metadataSuffix)
	if descriptionWidth < 1 {
		metadataSuffix = ""
		descriptionWidth = options.Width - lipgloss.Width(prefix)
	}
	if descriptionWidth < 1 {
		descriptionWidth = 1
	}
	description := Truncate(task.Description, descriptionWidth)
	primary := PadRight(Truncate(prefix+description+metadataSuffix, options.Width), options.Width)

	if options.Density == DensityCompact || len(metadata) == 0 {
		return []string{renderTaskLine(primary, options.Width, options.Selected, options.Styles)}
	}

	metadataPrefix := strings.Repeat(" ", lipgloss.Width(prefix))
	if options.Selected {
		metadataPrefix = gutter + strings.Repeat(" ", lipgloss.Width(icon+" "))
	}
	metadataLine := metadataPrefix + strings.Join(metadata, "  ·  ")
	metadataLine = PadRight(Truncate(metadataLine, options.Width), options.Width)
	return []string{
		renderTaskLine(primary, options.Width, options.Selected, options.Styles),
		renderTaskLine(metadataLine, options.Width, options.Selected, options.Styles),
	}
}

func renderTaskLine(line string, width int, selected bool, styles Styles) string {
	line = PadRight(Truncate(line, width), width)
	if selected {
		return styles.Selection.Width(width).Render(line)
	}
	return line
}

func selectionMarker(icons Icons) string {
	if icons.Pending == "[ ]" {
		return ">"
	}
	return "▌"
}

func taskMetadata(task domain.Task, options TaskRowOptions) []string {
	metadata := make([]string, 0, 4)
	if !options.HideProject && task.Project != "" {
		metadata = append(metadata, options.Styles.Project.Render("#"+task.Project))
	}
	if task.Due != nil {
		date := "Due " + formatDate(task.Due.In(options.Now.Location()), options.Now)
		if ClassifyOverdue(task, options.Now) {
			date = "Overdue · " + date
			metadata = append(metadata, options.Styles.Overdue.Render(date))
		} else {
			metadata = append(metadata, options.Styles.Metadata.Render(date))
		}
	}
	if task.Scheduled != nil {
		date := "Scheduled " + formatDate(task.Scheduled.In(options.Now.Location()), options.Now)
		metadata = append(metadata, options.Styles.Metadata.Render(date))
	}
	if task.Priority != "" {
		metadata = append(metadata, priorityStyle(task.Priority, options.Styles).Render(PriorityLabel(task.Priority)))
	}
	return metadata
}

func compactMetadata(task domain.Task, options TaskRowOptions) []string {
	// Compact metadata is opt-in because the description is the primary
	// affordance at 28–49 columns. Keep one short state/date/priority cue;
	// full timestamps remain on the comfortable metadata line and details
	// remain available through Enter.
	candidate := ""
	if ClassifyOverdue(task, options.Now) {
		candidate = "Overdue"
	} else if task.Due != nil {
		candidate = "Due " + compactDate(task.Due.In(options.Now.Location()), options.Now)
	} else if task.Scheduled != nil {
		candidate = "Scheduled " + compactDate(task.Scheduled.In(options.Now.Location()), options.Now)
	} else if task.Priority != "" {
		candidate = PriorityLabel(task.Priority)
	}
	if candidate == "" {
		return nil
	}
	prefixWidth := lipgloss.Width(options.Icons.Pending + " ")
	if options.Selected {
		prefixWidth = lipgloss.Width(selectionMarker(options.Icons) + " " + options.Icons.Pending + " ")
	}
	// The candidate shares the primary line with a five-cell separator. Keep
	// it only when it leaves at least one cell for the description.
	if prefixWidth+5+lipgloss.Width(candidate) >= options.Width {
		return nil
	}
	return []string{options.Styles.Metadata.Render(candidate)}
}

func priorityStyle(priority string, styles Styles) lipgloss.Style {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case "H", "HIGH":
		return styles.PriorityHigh
	case "M", "MEDIUM":
		return styles.PriorityMed
	case "L", "LOW":
		return styles.PriorityLow
	default:
		return styles.Priority
	}
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

func formatDate(value, now time.Time) string {
	loc := now.Location()
	today := now.In(loc)
	value = value.In(loc)
	if value.Year() == today.Year() && value.YearDay() == today.YearDay() {
		return "Today " + value.Format("15:04")
	}
	tomorrow := today.AddDate(0, 0, 1)
	if value.Year() == tomorrow.Year() && value.YearDay() == tomorrow.YearDay() {
		return "Tomorrow " + value.Format("15:04")
	}
	return value.Format("Jan 2 15:04")
}

func compactDate(value, now time.Time) string {
	parts := strings.Fields(formatDate(value, now))
	if len(parts) == 0 {
		return ""
	}
	if parts[0] == "Today" || parts[0] == "Tomorrow" || parts[0] == "Yesterday" {
		return parts[0]
	}
	if len(parts) > 1 {
		return parts[0] + " " + parts[1]
	}
	return parts[0]
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
