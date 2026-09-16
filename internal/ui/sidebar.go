package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// NavItem is a view navigation entry.
type NavItem struct {
	Key       string
	Label     string
	Count     int
	Icon      string
	HideCount bool
}

// RenderSidebar renders a borderless wide-layout navigation rail.
func RenderSidebar(active string, items []NavItem, width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := make([]string, 0, height)
	for _, item := range items {
		line := navItemText(item)
		if !item.HideCount {
			count := fmt.Sprintf("%d", item.Count)
			available := width - lipgloss.Width(count) - 1
			if available < 1 {
				available = 1
			}
			line = Truncate(line, available) + " " + count
		} else {
			line = Truncate(line, width)
		}
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
		label := navItemText(item)
		if !item.HideCount {
			label = fmt.Sprintf("%s %d", label, item.Count)
		}
		if strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label) {
			parts = append(parts, styles.Selection.Render(label))
		} else {
			parts = append(parts, styles.Muted.Render(label))
		}
	}
	line := strings.Join(parts, "   ")
	return Truncate(line, width)
}

// TabIndexAt returns the navigation item occupying x in the rendered compact
// tab row, or -1 for gaps and truncated-away space.
func TabIndexAt(items []NavItem, width, x int) int {
	if width <= 0 || x < 0 || x >= width {
		return -1
	}
	at := 0
	for index, item := range items {
		itemWidth := navItemWidth(item)
		if at >= width {
			break
		}
		visible := min(itemWidth, width-at)
		if x >= at && x < at+visible {
			return index
		}
		at += itemWidth
		if index < len(items)-1 {
			at += 3
		}
	}
	return -1
}

func navItemText(item NavItem) string {
	return fmt.Sprintf("%s %s", item.Icon, item.Label)
}

func navItemWidth(item NavItem) int {
	width := lipgloss.Width(navItemText(item))
	if !item.HideCount {
		width += lipgloss.Width(fmt.Sprintf(" %d", item.Count))
	}
	return width
}
