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
	Width        int
	Height       int
	Selected     int
	ShowMetadata bool
	Now          time.Time
	Styles       Styles
	Icons        Icons
}

// TaskRowOptions controls one compact row.
type TaskRowOptions struct {
	Width        int
	ShowMetadata bool
	Selected     bool
	Now          time.Time
	Styles       Styles
	Icons        Icons
}

// RenderTaskList renders only the visible window of rows. The selected row is
// kept inside that window, so scrolling never hides the current selection.
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
	start, end := VisibleTaskRange(len(tasks), selected, options.Height)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		lines = append(lines, RenderTaskRow(tasks[index], TaskRowOptions{
			Width:        options.Width,
			ShowMetadata: options.ShowMetadata,
			Selected:     index == selected,
			Now:          options.Now,
			Styles:       options.Styles,
			Icons:        options.Icons,
		}))
	}
	return strings.Join(lines, "\n")
}

// VisibleTaskRange calculates a scrolling window containing selected.
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

// RenderTaskRow renders completion, description, project, relevant dates,
// priority, and active state while never exposing raw urgency by default.
func RenderTaskRow(task domain.Task, options TaskRowOptions) string {
	if options.Width <= 0 {
		return ""
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	icon := options.Icons.Pending
	if task.Start != nil {
		icon = options.Icons.Active
	} else if !task.IsPending() {
		icon = options.Icons.Completed
	}
	prefix := icon + " "
	if options.ShowMetadata {
		prefix = PadRight(prefix, 3)
	}

	metadata := make([]string, 0, 3)
	if options.ShowMetadata && task.Project != "" {
		metadata = append(metadata, options.Styles.Project.Render("#"+task.Project))
	}
	if options.ShowMetadata {
		if date := relevantDate(task, options.Now); date != "" {
			if ClassifyOverdue(task, options.Now) {
				metadata = append(metadata, options.Styles.Overdue.Render(date))
			} else {
				metadata = append(metadata, options.Styles.Muted.Render(date))
			}
		}
		if task.Priority != "" {
			metadata = append(metadata, options.Styles.Priority.Render(task.Priority))
		}
	}

	separator := "  "
	metadataWidth := 0
	if len(metadata) > 0 {
		for _, value := range metadata {
			metadataWidth += lipgloss.Width(value)
		}
		metadataWidth += (len(metadata) - 1) * lipgloss.Width(separator)
		metadataWidth += lipgloss.Width(separator)
	}
	descriptionWidth := options.Width - lipgloss.Width(prefix) - metadataWidth
	if descriptionWidth < 1 {
		descriptionWidth = 1
	}
	description := Truncate(task.Description, descriptionWidth)
	line := prefix + description
	if len(metadata) > 0 {
		line += separator + strings.Join(metadata, separator)
	}
	line = Truncate(line, options.Width)
	line = PadRight(line, options.Width)
	if options.Selected {
		return options.Styles.Selection.Width(options.Width).Render(line)
	}
	return line
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
	if value.Year() == now.Year() && value.YearDay() == now.YearDay() {
		return "Today " + value.Format("15:04")
	}
	return value.Format("Jan 2 15:04")
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
