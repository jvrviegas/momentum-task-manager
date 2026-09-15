package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// NavItem is a view navigation entry.
type NavItem struct {
	Key   string
	Label string
	Count int
	Icon  string
}

// RenderSidebar renders a borderless wide-layout navigation rail.
func RenderSidebar(active string, items []NavItem, width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := make([]string, 0, height)
	for _, item := range items {
		label := fmt.Sprintf("%s %s", item.Icon, item.Label)
		count := fmt.Sprintf("%d", item.Count)
		available := width - lipgloss.Width(count) - 1
		if available < 1 {
			available = 1
		}
		line := Truncate(label, available) + " " + count
		line = PadRight(line, width)
		if strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label) {
			line = styles.Selection.Width(width).Render(line)
		} else {
			line = styles.Muted.Render(line)
		}
		lines = append(lines, line)
		if len(lines) == height {
			break
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// RenderTabs renders the compact top navigation used below the wide
// breakpoint.
func RenderTabs(active string, items []NavItem, width int, styles Styles) string {
	if width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		label := fmt.Sprintf("%s %s %d", item.Icon, item.Label, item.Count)
		if strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label) {
			parts = append(parts, styles.Selection.Render(label))
		} else {
			parts = append(parts, styles.Muted.Render(label))
		}
	}
	line := strings.Join(parts, "   ")
	return Truncate(line, width)
}
