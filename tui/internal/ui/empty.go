package ui

import (
	"fmt"
	"strings"
)

const (
	InboxEmptyTitle     = "No pending tasks"
	InboxEmptyBody      = "Tasks that are not due or scheduled today land here."
	TodayEmptyTitle     = "Nothing due today"
	TodayEmptyBody      = "Overdue, due and scheduled tasks land here."
	CompletedEmptyTitle = "No recently completed tasks"
	CompletedEmptyBody  = "Tasks completed in the last 30 days appear here."
)

// EmptyOptions describes an empty view body. InboxCount feeds the Today
// state's “open Inbox · n waiting” hint.
type EmptyOptions struct {
	View       string
	Width      int
	Height     int
	InboxCount int
	Styles     Styles
	Icons      Icons
}

// RenderEmptyState is calm, specific and actionable: what would appear here
// and the keys that get you somewhere, left-aligned at the title column.
func RenderEmptyState(options EmptyOptions) string {
	width := options.Width
	if width <= 0 {
		return ""
	}
	icons := options.Icons.orUnicode()
	styles := options.Styles
	icon, title, body := icons.Inbox, InboxEmptyTitle, InboxEmptyBody
	hints := []Hint{{"c", "capture a task"}}
	switch {
	case strings.EqualFold(options.View, "today"):
		icon, title, body = icons.Today, TodayEmptyTitle, TodayEmptyBody
		hints = append(hints, Hint{"1", fmt.Sprintf("open Inbox %s %d waiting", icons.Dot, options.InboxCount)})
	case strings.EqualFold(options.View, "completed"):
		icon, title, body = icons.CompletedView, CompletedEmptyTitle, CompletedEmptyBody
		hints = append(hints, Hint{"2", "open Today"})
	}
	heading := []Span{txt("  ")}
	if icon != "" {
		heading = append(heading, muted(icon), txt(" "))
	}
	heading = append(heading, txt(title).bold())
	lines := []string{styles.Line(width, FillNone, heading...)}
	for _, line := range WrapText(body, max(1, width-4)) {
		lines = append(lines, styles.Line(width, FillNone, gap(4), muted(line)))
	}
	lines = append(lines, "")
	chips := []Span{gap(4)}
	for index, hint := range hints {
		next := []Span{txt(" " + hint.Key + " ").bold().on(FillPanel), muted(" " + hint.Label)}
		if index > 0 && spansWidth(chips)+4+spansWidth(next) > width {
			lines = append(lines, styles.Line(width, FillNone, chips...))
			chips = []Span{gap(4)}
		} else if index > 0 {
			chips = append(chips, gap(4))
		}
		chips = append(chips, next...)
	}
	lines = append(lines, styles.Line(width, FillNone, chips...))
	for lead := 0; lead < 3 && (options.Height <= 0 || len(lines) < options.Height); lead++ {
		lines = append([]string{""}, lines...)
	}
	return strings.Join(lines, "\n")
}

// RenderEmpty renders the approved copy for a fixed view.
func RenderEmpty(view string, width int, styles Styles) string {
	return RenderEmptyState(EmptyOptions{View: view, Width: width, Styles: styles})
}

func RenderInboxEmpty(width int, styles Styles) string { return RenderEmpty("inbox", width, styles) }
func RenderTodayEmpty(width int, styles Styles) string { return RenderEmpty("today", width, styles) }
