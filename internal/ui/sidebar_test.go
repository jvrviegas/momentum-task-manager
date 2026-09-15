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
