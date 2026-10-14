package ui

import (
	"fmt"
	"strings"
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

type sidebarRowKind int

const (
	sidebarBlank sidebarRowKind = iota
	sidebarBrand
	sidebarGroup
	sidebarItem
)

type sidebarRow struct {
	kind      sidebarRowKind
	itemIndex int
	group     string
}

// RenderSidebar renders the wide navigation rail: app name, Muted caps group
// labels and nav items. The │ divider is drawn by the compositor.
func RenderSidebar(active string, items []NavItem, width, height int, styles Styles, icons Icons) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	icons = icons.orUnicode()
	rows := sidebarRows(items, height)
	lines := make([]string, 0, height)
	for _, row := range rows {
		switch row.kind {
		case sidebarBrand:
			lines = append(lines, styles.Line(width, FillNone, txt(" "), sp("Momentum", ToneAccent).bold()))
		case sidebarGroup:
			lines = append(lines, styles.Line(width, FillNone, txt(" "), muted(strings.ToUpper(row.group))))
		case sidebarItem:
			lines = append(lines, renderSidebarItem(active, items[row.itemIndex], width, styles, icons))
		default:
			lines = append(lines, spaces(width))
		}
	}
	for len(lines) < height {
		lines = append(lines, spaces(width))
	}
	return strings.Join(lines, "\n")
}

func navActive(active string, item NavItem) bool {
	return strings.EqualFold(active, item.Key) || strings.EqualFold(active, item.Label)
}

func renderSidebarItem(active string, item NavItem, width int, styles Styles, icons Icons) string {
	count := ""
	if !item.HideCount {
		count = fmt.Sprintf("%d", item.Count)
	}
	if navActive(active, item) {
		spans := []Span{sp(icons.Selection, ToneAccent), txt(" ")}
		if item.Icon != "" {
			spans = append(spans, sp(item.Icon, ToneAccent), txt(" "))
		}
		spans = append(spans, txt(item.Label).bold(), grow(), txt(count), txt(" "))
		return styles.Line(width, FillSelection, spans...)
	}
	spans := []Span{txt("  ")}
	if item.Icon != "" {
		spans = append(spans, muted(item.Icon), txt(" "))
	}
	spans = append(spans, txt(item.Label), grow(), muted(count), txt(" "))
	return styles.Line(width, FillNone, spans...)
}

// sidebarRows returns the shared render and hit-test geometry. A full rail
// has the app name, a blank row, group headings and a blank row between
// groups. Constrained rails drop the group gap, the blank under the name,
// the headings and then the name before hiding navigation items.
func sidebarRows(items []NavItem, height int) []sidebarRow {
	if height <= 0 {
		return nil
	}
	build := func(brand, brandGap, headings, groupGap bool) []sidebarRow {
		rows := make([]sidebarRow, 0, len(items)+6)
		if brand {
			rows = append(rows, sidebarRow{kind: sidebarBrand, itemIndex: -1})
			if brandGap {
				rows = append(rows, sidebarRow{kind: sidebarBlank, itemIndex: -1})
			}
		}
		lastGroup := ""
		for index, item := range items {
			group := strings.TrimSpace(item.Group)
			if group != "" && !strings.EqualFold(group, lastGroup) && headings {
				if groupGap && lastGroup != "" {
					rows = append(rows, sidebarRow{kind: sidebarBlank, itemIndex: -1})
				}
				rows = append(rows, sidebarRow{kind: sidebarGroup, itemIndex: -1, group: group})
			}
			rows = append(rows, sidebarRow{kind: sidebarItem, itemIndex: index})
			lastGroup = group
		}
		return rows
	}
	variants := [][4]bool{
		{true, true, true, true},
		{true, true, true, false},
		{true, false, true, false},
		{true, false, false, false},
		{false, false, false, false},
	}
	var rows []sidebarRow
	for _, v := range variants {
		rows = build(v[0], v[1], v[2], v[3])
		if len(rows) <= height {
			return rows
		}
	}
	return rows[:height]
}

// SidebarItemAt returns the navigation item occupying y in the rendered rail.
// Name, heading and separator rows deliberately have no click target.
func SidebarItemAt(items []NavItem, height, y int) int {
	if y < 0 || y >= height {
		return -1
	}
	rows := sidebarRows(items, height)
	if y >= len(rows) || rows[y].kind != sidebarItem {
		return -1
	}
	return rows[y].itemIndex
}

type tabLayoutResult struct {
	spans  []Span
	hits   [][2]int // per item, [start, end) columns; empty when not clickable
	active [2]int   // underline columns, inclusive
}

// tabLayout chooses the widest tab row variant that fits: app name and
// labels, labels only, icon + count for inactive tabs, then only the active
// tab plus • · · · position dots.
func tabLayout(active string, items []NavItem, width int, icons Icons) tabLayoutResult {
	type variant struct {
		brand  bool
		labels bool
		gap    int
	}
	var variants []variant
	if width >= NarrowBreakpoint {
		variants = append(variants, variant{true, true, 3}, variant{false, true, 3})
	}
	if width >= MinimalTabsBreakpoint {
		variants = append(variants, variant{false, false, 2})
	}
	for _, v := range variants {
		result := tabRow(active, items, icons, v.brand, v.labels, v.gap)
		if spansWidth(result.spans) <= width {
			return result
		}
	}
	return minimalTabs(active, items, width, icons)
}

func tabRow(active string, items []NavItem, icons Icons, brand, labels bool, gapCells int) tabLayoutResult {
	result := tabLayoutResult{hits: make([][2]int, len(items)), active: [2]int{-1, -1}}
	spans := []Span{txt(" ")}
	x := 1
	if brand {
		spans = append(spans, sp("Momentum", ToneAccent).bold(), gap(4))
		x += 12
	}
	for index, item := range items {
		if index > 0 {
			spans = append(spans, gap(gapCells))
			x += gapCells
		}
		count := ""
		if !item.HideCount {
			count = fmt.Sprintf("%d", item.Count)
		}
		var tab []Span
		switch {
		case navActive(active, item):
			if item.Icon != "" {
				tab = append(tab, sp(item.Icon, ToneAccent), txt(" "))
			}
			tab = append(tab, txt(item.Label).bold())
			if count != "" {
				tab = append(tab, txt(" "+count))
			}
		case labels:
			if item.Icon != "" {
				tab = append(tab, muted(item.Icon+" "))
			}
			tab = append(tab, muted(item.Label))
			if count != "" {
				tab = append(tab, muted(" "+count))
			}
		default:
			icon := item.Icon
			if icon == "" {
				icon = string([]rune(item.Label)[:1])
			}
			tab = append(tab, muted(icon))
			if count != "" {
				tab = append(tab, muted(" "+count))
			}
		}
		w := spansWidth(tab)
		result.hits[index] = [2]int{x, x + w}
		if navActive(active, item) {
			result.active = [2]int{x, x + w - 1}
		}
		spans = append(spans, tab...)
		x += w
	}
	result.spans = spans
	return result
}

func minimalTabs(active string, items []NavItem, width int, icons Icons) tabLayoutResult {
	result := tabLayoutResult{hits: make([][2]int, len(items)), active: [2]int{-1, -1}}
	current := 0
	for index, item := range items {
		if navActive(active, item) {
			current = index
		}
	}
	if len(items) == 0 {
		return result
	}
	item := items[current]
	lead := []Span{txt(" ")}
	if item.Icon != "" {
		lead = append(lead, sp(item.Icon, ToneAccent), txt(" "))
	}
	lead = append(lead, txt(item.Label).bold())
	if !item.HideCount {
		lead = append(lead, txt(fmt.Sprintf(" %d", item.Count)))
	}
	leadWidth := spansWidth(lead)
	dotsLeft := width - 1 - len(items)
	if leadWidth > dotsLeft {
		leadWidth = max(1, dotsLeft)
	}
	result.active = [2]int{1, leadWidth - 1}
	result.hits[current] = [2]int{1, leadWidth}
	spans := append(lead, grow())
	for index := range items {
		x := dotsLeft + index
		if index == current {
			spans = append(spans, sp(icons.TabOn, ToneAccent))
		} else {
			spans = append(spans, muted(icons.TabOff))
			result.hits[index] = [2]int{x, x + 1}
		}
	}
	result.spans = append(spans, txt(" "))
	return result
}

// RenderTabs renders the compact navigation used below the wide breakpoint:
// a tab row and a ─ rule with ━ under exactly the active tab's cells.
func RenderTabs(active string, items []NavItem, width int, styles Styles, icons Icons) string {
	if width <= 0 {
		return ""
	}
	icons = icons.orUnicode()
	layout := tabLayout(active, items, width, icons)
	top := styles.Line(width, FillNone, layout.spans...)
	if width < 3 {
		return top
	}
	inner := width - 2
	from := min(max(0, layout.active[0]-1), inner)
	to := min(max(from, layout.active[1]), inner)
	under := []Span{
		txt(" "),
		sp(strings.Repeat(icons.Rule, from), ToneBorder),
		sp(strings.Repeat(icons.RuleActive, to-from), ToneAccent),
		sp(strings.Repeat(icons.Rule, inner-to), ToneBorder),
		txt(" "),
	}
	return top + "\n" + styles.Line(width, FillNone, under...)
}

// TabIndexAt returns the navigation item occupying x in the rendered tab
// row, or -1 for gaps. It uses the same variant choice as RenderTabs.
func TabIndexAt(active string, items []NavItem, width, x int, icons Icons) int {
	if width <= 0 || x < 0 || x >= width {
		return -1
	}
	layout := tabLayout(active, items, width, icons.orUnicode())
	for index, hit := range layout.hits {
		if hit[1] > hit[0] && x >= hit[0] && x < hit[1] {
			return index
		}
	}
	return -1
}
