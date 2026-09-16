package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestNavigationCanHideTaskCountForSettings(t *testing.T) {
	items := []NavItem{
		{Key: "inbox", Label: "Inbox", Count: 4, Icon: "I"},
		{Key: "today", Label: "Today", Count: 2, Icon: "D"},
		{Key: "settings", Label: "Settings", Count: 0, Icon: "S", HideCount: true},
	}
	styles := NewStyles(ResolveTheme("dark", true))
	sidebar := RenderSidebar("settings", items, 24, 4, styles)
	lines := strings.Split(sidebar, "\n")
	if len(lines) < 3 || !strings.Contains(lines[2], "Settings") || strings.Contains(lines[2], "Settings 0") || lipgloss.Width(lines[2]) != 24 {
		t.Fatalf("sidebar=%q", sidebar)
	}
	tabs := RenderTabs("settings", items, 80, styles)
	if !strings.Contains(tabs, "Settings") || strings.Contains(tabs, "Settings 0") {
		t.Fatalf("tabs=%q", tabs)
	}
}

func TestTabIndexAtUsesRenderedItemBoundaries(t *testing.T) {
	items := []NavItem{
		{Key: "inbox", Label: "Inbox", Count: 4, Icon: "I"},
		{Key: "today", Label: "Today", Count: 2, Icon: "D"},
		{Key: "settings", Label: "Settings", HideCount: true, Icon: "S"},
	}
	for index, want := range []string{"inbox", "today", "settings"} {
		x := navigationItemStart(items, 80, index)
		if got := TabIndexAt(items, 80, x); got != index || items[got].Key != want {
			t.Fatalf("index=%d x=%d got=%d", index, x, got)
		}
	}
	if got := TabIndexAt(items, 80, 79); got != -1 {
		t.Fatalf("truncated/outside tab got=%d", got)
	}
}

func navigationItemStart(items []NavItem, width, index int) int {
	start := 0
	for i := 0; i < index; i++ {
		start += navItemWidth(items[i]) + 3
	}
	return start
}
