package ui

import "strings"

const (
	InboxEmptyTitle = "No pending tasks"
	InboxEmptyHint  = "Ctrl+K to capture something"
	TodayEmptyTitle = "Nothing scheduled for today"
	TodayEmptyBody  = "Your day is clear."
	TodayEmptyHint  = "Ctrl+K to add a task"
)

// RenderEmpty renders approved copy for a fixed view.
func RenderEmpty(view string, width int, styles Styles) string {
	var lines []string
	if strings.EqualFold(view, "today") {
		lines = []string{TodayEmptyTitle, TodayEmptyBody, TodayEmptyHint}
	} else {
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
