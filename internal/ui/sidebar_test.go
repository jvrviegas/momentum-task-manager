package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func navItems() []NavItem {
	return []NavItem{{Key: "inbox", Label: "Inbox", Count: 4, Icon: "I"}, {Key: "today", Label: "Today", Count: 2, Icon: "D"}}
}

func TestRenderSidebarShowsCountsAndActiveView(t *testing.T) {
	got := RenderSidebar("today", navItems(), 24, 4, NewStyles(ResolveTheme("dark", true)))
	if !strings.Contains(got, "Inbox") || !strings.Contains(got, "Today") || !strings.Contains(got, "4") || !strings.Contains(got, "2") {
		t.Fatalf("sidebar=%q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 4 || lipgloss.Width(lines[1]) != 24 {
		t.Fatalf("sidebar lines=%#v", lines)
	}
}

func TestRenderSidebarSeparatesGroupHeadingsAndClickRows(t *testing.T) {
	items := []NavItem{
		{Key: "inbox", Label: "Inbox", Count: 43, Icon: "I", Group: "Views"},
		{Key: "today", Label: "Today", Count: 1, Icon: "D", Group: "Views"},
		{Key: "settings", Label: "Settings", Icon: "S", HideCount: true, Group: "Manage"},
	}
	lines := strings.Split(RenderSidebar("inbox", items, 24, 8, Styles{}), "\n")
	if !strings.Contains(lines[0], "VIEWS") || strings.Contains(lines[0], "Inbox") || !strings.Contains(lines[1], "Inbox 43") || !strings.Contains(lines[2], "Today 1") {
		t.Fatalf("views group lines=%q", lines[:3])
	}
	if strings.TrimSpace(lines[3]) != "" || !strings.Contains(lines[4], "MANAGE") || strings.Contains(lines[4], "Settings") || !strings.Contains(lines[5], "Settings") {
		t.Fatalf("manage group lines=%q", lines[3:6])
	}
	for y, want := range []int{-1, 0, 1, -1, -1, 2, -1, -1} {
		if got := SidebarItemAt(items, 8, y); got != want {
			t.Fatalf("row %d: item=%d want=%d", y, got, want)
		}
	}
}

func TestRenderSidebarAcceptsKeyOrLabelAsActive(t *testing.T) {
	byKey := RenderSidebar("inbox", navItems(), 20, 2, NewStyles(ResolveTheme("dark", true)))
	byLabel := RenderSidebar("Today", navItems(), 20, 2, NewStyles(ResolveTheme("dark", true)))
	if lipgloss.Width(strings.Split(byKey, "\n")[0]) != 20 || lipgloss.Width(strings.Split(byLabel, "\n")[1]) != 20 {
		t.Fatal("active rows were not full width")
	}
}

func TestRenderSidebarHandlesSmallDimensions(t *testing.T) {
	if RenderSidebar("inbox", navItems(), 0, 2, Styles{}) != "" || RenderSidebar("inbox", navItems(), 20, 0, Styles{}) != "" {
		t.Fatal("invalid dimensions should be empty")
	}
}

func TestRenderTabsShowsCompactNavigation(t *testing.T) {
	got := RenderTabs("inbox", navItems(), 80, NewStyles(ResolveTheme("dark", true)))
	if !strings.Contains(got, "Inbox") || !strings.Contains(got, "Today") || !strings.Contains(got, "4") {
		t.Fatalf("tabs=%q", got)
	}
	if lipgloss.Width(got) > 80 {
		t.Fatalf("tabs width=%d", lipgloss.Width(got))
	}
}
