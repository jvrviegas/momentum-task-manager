package ui

import "strings"

const (
	InboxEmptyTitle     = "No pending tasks"
	InboxEmptyHint      = "Ctrl+K to capture something"
	TodayEmptyTitle     = "Nothing scheduled for today"
	TodayEmptyBody      = "Your day is clear."
	TodayEmptyHint      = "Ctrl+K to add a task"
	CompletedEmptyTitle = "No recently completed tasks"
	CompletedEmptyBody  = "Tasks completed in the last 30 days appear here."
	CompletedEmptyHint  = "Ctrl+K to add a task"
)

// RenderEmpty renders approved copy for a fixed view.
func RenderEmpty(view string, width int, styles Styles) string {
	var lines []string
	switch {
	case strings.EqualFold(view, "today"):
		lines = []string{TodayEmptyTitle, TodayEmptyBody, TodayEmptyHint}
	case strings.EqualFold(view, "completed"):
		lines = []string{CompletedEmptyTitle, CompletedEmptyBody, CompletedEmptyHint}
	default:
		lines = []string{InboxEmptyTitle, InboxEmptyHint}
	}
	if width <= 0 {
		return ""
	}
	for index, line := range lines {
		lines[index] = styles.ModalBody.Width(width).Render(Truncate(line, width))
	}
	return strings.Join(lines, "\n")
}

func RenderInboxEmpty(width int, styles Styles) string { return RenderEmpty("inbox", width, styles) }
func RenderTodayEmpty(width int, styles Styles) string { return RenderEmpty("today", width, styles) }
