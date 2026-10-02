package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func renderOptions(width, height, selected int) TaskListOptions {
	return TaskListOptions{
		Width: width, Height: height, Selected: selected, ShowMetadata: true,
		Now:    time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
		Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("unicode"),
	}
}

func TestRenderTaskRowIncludesSupportedDefaultMetadata(t *testing.T) {
	due := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	task := domain.Task{Description: "Finish API proposal", Status: "pending", Project: "work.client", Priority: "H", Due: &due}
	got := RenderTaskRow(task, TaskRowOptions{Width: 80, ShowMetadata: true, Now: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC), Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("unicode")})
	for _, want := range []string{"Finish API proposal", "#work.client", "Today 17:00", "H"} {
		if !strings.Contains(got, want) {
			t.Errorf("row %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "12.3") {
		t.Error("urgency should not be rendered")
	}
	if lipgloss.Width(got) != 80 {
		t.Fatalf("display width=%d", lipgloss.Width(got))
	}
}

func TestRenderTaskRowUsesActiveAndCompletedIndicators(t *testing.T) {
	start := time.Now()
	options := TaskRowOptions{Width: 30, Now: time.Now(), Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("ascii")}
	if got := sanitizeComponentRender(RenderTaskRow(domain.Task{Description: "active", Status: "pending", Start: &start}, options)); !strings.HasPrefix(got, "  [>] active") {
		t.Fatalf("active row=%q", got)
	}
	if got := sanitizeComponentRender(RenderTaskRow(domain.Task{Description: "done", Status: "completed"}, options)); !strings.HasPrefix(got, "  [x] done") {
		t.Fatalf("completed row=%q", got)
	}
}

func TestRenderTaskRowHidesMetadataAtNarrowWidth(t *testing.T) {
	due := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	task := domain.Task{Description: "Keep description visible", Status: "pending", Project: "work", Priority: "H", Due: &due}
	got := RenderTaskRow(task, TaskRowOptions{Width: 35, ShowMetadata: false, Now: time.Now(), Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("unicode")})
	if strings.Contains(got, "#work") || strings.Contains(got, "Today") || strings.Contains(got, " H") {
		t.Fatalf("metadata leaked into narrow row=%q", got)
	}
	if lipgloss.Width(got) != 35 {
		t.Fatalf("width=%d", lipgloss.Width(got))
	}
}

func TestOverdueUsesDueOnly(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	scheduled := now.Add(-24 * time.Hour)
	if ClassifyOverdue(domain.Task{Scheduled: &scheduled}, now) {
		t.Fatal("old scheduled date must not be overdue")
	}
	due := now.Add(-24 * time.Hour)
	if !ClassifyOverdue(domain.Task{Due: &due}, now) {
		t.Fatal("old due date should be overdue")
	}
}

func TestVisibleTaskRangeKeepsSelectionVisible(t *testing.T) {
	cases := []struct {
		count, selected, height int
		start, end              int
	}{
		{10, 0, 3, 0, 3},
		{10, 5, 3, 3, 6},
		{10, 9, 3, 7, 10},
		{2, 1, 5, 0, 2},
	}
	for _, tc := range cases {
		start, end := VisibleTaskRange(tc.count, tc.selected, tc.height)
		if start != tc.start || end != tc.end || tc.selected < start || tc.selected >= end {
			t.Errorf("case=%#v got=%d,%d", tc, start, end)
		}
	}
}

func TestRenderTaskListScrollsAndSelectsFullRow(t *testing.T) {
	tasks := make([]domain.Task, 5)
	for i := range tasks {
		tasks[i] = domain.Task{UUID: string(rune('a' + i)), Description: "task " + string(rune('a'+i)), Status: "pending"}
	}
	got := RenderTaskList(tasks, renderOptions(24, 2, 3))
	if !strings.Contains(got, "task c") || !strings.Contains(got, "task d") || strings.Contains(got, "task a") {
		t.Fatalf("scroll output=%q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 2 || lipgloss.Width(lines[0]) != 24 || lipgloss.Width(lines[1]) != 24 {
		t.Fatalf("lines=%#v", lines)
	}
}

func TestRowSummaryIsPlainAndUseful(t *testing.T) {
	got := RowSummary(domain.Task{Description: "Task", Status: "pending", Project: "work"}, time.Now())
	if got != "Task [pending] #work" {
		t.Fatalf("summary=%q", got)
	}
}
