package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// NavItem is a view navigation entry. Group is rendered as a standalone
// section heading when the rail has enough vertical space.
type NavItem struct {
	Key       string
	Label     string
	Count     int
	Icon      string
	HideCount bool
	Group     string
}

type sidebarRow struct {
	itemIndex int
	group     string
}

// RenderSidebar renders a borderless wide-layout navigation rail.
func RenderSidebar(active string, items []NavItem, width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	rows := sidebarRows(items, height)
	lines := make([]string, 0, height)
	for _, row := range rows {
		if row.itemIndex >= 0 {
			lines = append(lines, renderSidebarItem(active, items[row.itemIndex], width, styles))
			continue
		}
		if row.group != "" {
			lines = append(lines, renderSidebarGroup(row.group, width, styles))
			continue
		}
		lines = append(lines, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func renderSidebarGroup(group string, width int, styles Styles) string {
	line := PadRight(Truncate(strings.ToUpper(strings.TrimSpace(group)), width), width)
	return styles.Muted.Bold(true).Render(line)
}

func renderSidebarItem(active string, item NavItem, width int, styles Styles) string {
	text := navItemText(item)
	if !item.HideCount {
		text += " " + fmt.Sprintf("%d", item.Count)
	}
	marker := "  "
	isActive := strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label)
	if isActive {
		marker = navSelectionMarker(item) + " "
	}
	line := PadRight(Truncate(marker+text, width), width)
	if isActive {
		return styles.Selection.Width(width).Render(line)
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

// sidebarRows returns the shared render and hit-test geometry. Full-height
// rails get standalone headings and a blank row between groups. Constrained
// rails omit the blank row, then headings, before hiding navigation items.
func sidebarRows(items []NavItem, height int) []sidebarRow {
	if height <= 0 {
		return nil
	}
	groupCount := 0
	lastGroup := ""
	for _, item := range items {
		group := strings.TrimSpace(item.Group)
		if group != "" && !strings.EqualFold(group, lastGroup) {
			groupCount++
		}
		lastGroup = group
	}
	showHeadings := len(items)+groupCount <= height
	showGaps := showHeadings && len(items)+groupCount+max(groupCount-1, 0) <= height

	rows := make([]sidebarRow, 0, height)
	lastGroup = ""
	for index, item := range items {
		group := strings.TrimSpace(item.Group)
		groupStart := group != "" && !strings.EqualFold(group, lastGroup)
		if showHeadings && groupStart {
			if showGaps && len(rows) > 0 {
				rows = append(rows, sidebarRow{itemIndex: -1})
			}
			rows = append(rows, sidebarRow{itemIndex: -1, group: group})
		}
		rows = append(rows, sidebarRow{itemIndex: index})
		lastGroup = group
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

// SidebarItemAt returns the navigation item occupying y in the rendered rail.
// Heading and separator rows deliberately have no click target.
func SidebarItemAt(items []NavItem, height, y int) int {
	if y < 0 || y >= height {
		return -1
	}
	rows := sidebarRows(items, height)
	if y >= len(rows) {
		return -1
	}
	return rows[y].itemIndex
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
