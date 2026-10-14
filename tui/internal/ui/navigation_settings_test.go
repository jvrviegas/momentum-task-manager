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
	sidebar := RenderSidebar("settings", items, 24, 4, styles, Icons{})
	lines := strings.Split(sidebar, "\n")
	if len(lines) < 4 || !strings.Contains(lines[3], "Settings") || strings.Contains(lines[3], "Settings 0") || lipgloss.Width(lines[3]) != 24 {
		t.Fatalf("sidebar=%q", sidebar)
	}
	tabs := RenderTabs("settings", items, 80, styles, Icons{})
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
	for _, width := range []int{80, 45, 30} {
		row := strings.Split(componentANSI.ReplaceAllString(RenderTabs("today", items, width, Styles{}, Icons{}), ""), "\n")[0]
		for index, want := range []string{"inbox", "today", "settings"} {
			found := false
			for x := 0; x < width; x++ {
				if got := TabIndexAt("today", items, width, x, Icons{}); got == index && items[got].Key == want {
					found = strings.TrimSpace(string([]rune(row)[x])) != ""
					break
				}
			}
			if !found {
				t.Fatalf("width=%d tab %s has no rendered hit cell in %q", width, want, row)
			}
		}
		if got := TabIndexAt("today", items, width, 0, Icons{}); got != -1 {
			t.Fatalf("width=%d padding cell got=%d", width, got)
		}
	}
}
