package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestInboxEmptyCopy(t *testing.T) {
	got := RenderInboxEmpty(40, NewStyles(ResolveTheme("dark", true)))
	for _, want := range []string{InboxEmptyTitle, InboxEmptyHint} {
		if !strings.Contains(got, want) {
			t.Errorf("empty=%q missing %q", got, want)
		}
	}
}

func TestTodayEmptyCopy(t *testing.T) {
	got := RenderTodayEmpty(40, NewStyles(ResolveTheme("dark", true)))
	for _, want := range []string{TodayEmptyTitle, TodayEmptyBody, TodayEmptyHint} {
		if !strings.Contains(got, want) {
			t.Errorf("empty=%q missing %q", got, want)
		}
	}
}

func TestEmptyStateHandlesUnknownViewAsInbox(t *testing.T) {
	got := RenderEmpty("other", 30, Styles{})
	if strings.Contains(got, TodayEmptyTitle) || !strings.Contains(got, InboxEmptyTitle) {
		t.Fatalf("empty=%q", got)
	}
}

func TestEmptyStateIsWidthSafe(t *testing.T) {
	got := RenderTodayEmpty(12, Styles{})
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > 12 {
			t.Errorf("line width=%d line=%q", lipgloss.Width(line), line)
		}
	}
	if RenderTodayEmpty(0, Styles{}) != "" {
		t.Fatal("zero width should be empty")
	}
}
