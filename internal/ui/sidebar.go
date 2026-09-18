package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// NavItem is a view navigation entry. Group is rendered as a small inline
// section label on the first item in that group so the rail remains compact
// and its click rows stay one-to-one with navigation entries.
type NavItem struct {
	Key       string
	Label     string
	Count     int
	Icon      string
	HideCount bool
	Group     string
}

// RenderSidebar renders a borderless wide-layout navigation rail.
func RenderSidebar(active string, items []NavItem, width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := make([]string, 0, height)
	lastGroup := ""
	for _, item := range items {
		groupStart := item.Group != "" && !strings.EqualFold(item.Group, lastGroup)
		line := renderSidebarItem(active, item, groupStart, width, styles)
		lines = append(lines, line)
		lastGroup = item.Group
		if len(lines) == height {
			break
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func renderSidebarItem(active string, item NavItem, groupStart bool, width int, styles Styles) string {
	text := navItemText(item)
	if !item.HideCount {
		text += " " + fmt.Sprintf("%d", item.Count)
	}
	marker := "  "
	isActive := strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label)
	if isActive {
		marker = navSelectionMarker(item) + " "
	}
	group := ""
	if groupStart {
		group = strings.ToUpper(strings.TrimSpace(item.Group)) + "  "
	}
	line := group + marker + text
	line = Truncate(line, width)
	line = PadRight(line, width)
	if isActive {
		return styles.Selection.Width(width).Render(line)
	}
	if groupStart {
		// Keep the group heading legible without introducing a separate hit
		// row or consuming the compact rail's vertical budget.
		return styles.Muted.Render(line)
	}
	return styles.Muted.Render(line)
}

func navSelectionMarker(item NavItem) string {
	switch item.Icon {
	case "I", "D", "S", "/", "~", ">", "[ ]":
		return ">"
	default:
		return "▌"
	}
}

// SidebarItemAt returns the item occupying y in the rendered rail. Group
// labels are inline, therefore the result is stable regardless of grouping.
func SidebarItemAt(items []NavItem, height, y int) int {
	if y < 0 || y >= height || y >= len(items) {
		return -1
	}
	return y
}

// RenderTabs renders the compact top navigation used below the wide
// breakpoint. Active tabs retain their configured icon/text width and add a
// non-color underline/bold cue through the shared style.
func RenderTabs(active string, items []NavItem, width int, styles Styles) string {
	if width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label) {
			// Underline the configured icon as the non-color active cue,
			// while keeping the label as one contiguous styled string for
			// terminals and semantic render assertions.
			rest := " " + item.Label
			if !item.HideCount {
				rest += fmt.Sprintf(" %d", item.Count)
			}
			parts = append(parts, styles.Selection.Underline(true).Render(item.Icon)+styles.Selection.Bold(true).Render(rest))
			continue
		}
		label := navItemText(item)
		if !item.HideCount {
			label = fmt.Sprintf("%s %d", label, item.Count)
		}
		parts = append(parts, styles.Muted.Render(label))
	}
	line := strings.Join(parts, "   ")
	return Truncate(line, width)
}

// TabIndexAt returns the navigation item occupying x in the rendered compact
// tab row, or -1 for gaps and truncated-away space. Tab styling does not alter
// display width, so the same geometry is used by rendering and mouse input.
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
