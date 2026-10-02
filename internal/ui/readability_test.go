package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestComfortableTaskBlockSeparatesMetadataAndUsesLabels(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	due := now.Add(7 * time.Hour)
	row := RenderTaskRow(domain.Task{
		Description: "Review API proposal", Status: "pending", Project: "work", Priority: "H", Due: &due,
	}, TaskRowOptions{Width: 70, ShowMetadata: true, Density: DensityComfortable, Now: now, Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("unicode")})
	lines := strings.Split(row, "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "Review API proposal") || !strings.Contains(lines[1], "Due Today 17:00") || !strings.HasSuffix(strings.TrimRight(sanitizeComponentRender(lines[0]), " "), "↑ High") {
		t.Fatalf("row=%q", row)
	}
	if strings.Contains(lines[1], " H ") || strings.Contains(lines[1], " M ") || strings.Contains(lines[1], " L ") {
		t.Fatalf("raw priority code leaked: %q", row)
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 70 {
			t.Fatalf("line width=%d line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestNarrowTaskRowIsOneLineWithTrailingOverdueMarker(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	due := now.Add(7 * time.Hour)
	overdue := now.Add(-48 * time.Hour)
	options := TaskRowOptions{Width: 40, Density: DensityCompact, Now: now, Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("ascii")}
	row := RenderTaskRow(domain.Task{Description: "Review API", Status: "pending", Due: &due}, options)
	if strings.Contains(row, "\n") || strings.Contains(row, "Due") || strings.Contains(row, "!") {
		t.Fatalf("narrow row=%q", row)
	}
	late := sanitizeComponentRender(RenderTaskRow(domain.Task{Description: "Review API", Status: "pending", Due: &overdue}, options))
	if !strings.HasSuffix(late, " !") || !strings.HasPrefix(late, "  [ ] Review API") {
		t.Fatalf("overdue narrow row=%q", late)
	}
	if lipgloss.Width(row) != 40 {
		t.Fatalf("narrow width=%d", lipgloss.Width(row))
	}
}

func TestInboxTaskBlockCanOmitProjectMetadata(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	project := "work.client"
	row := RenderTaskRow(domain.Task{Description: "Task", Project: project, Status: "pending"}, TaskRowOptions{
		Width: 70, ShowMetadata: true, HideProject: true, Density: DensityComfortable, Now: now,
		Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor("unicode"),
	})
	if strings.Contains(row, "#"+project) {
		t.Fatalf("project repeated in inbox row=%q", row)
	}
}

func TestSelectedTaskBlockUsesNonColorGutterOnEveryLine(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	due := now.Add(time.Hour)
	for _, mode := range []string{"unicode", "ascii"} {
		row := RenderTaskRow(domain.Task{Description: "Selected", Status: "pending", Due: &due}, TaskRowOptions{
			Width: 40, ShowMetadata: true, Density: DensityComfortable, Selected: true, Now: now,
			Styles: NewStyles(ResolveTheme("dark", true)), Icons: IconsFor(mode),
		})
		lines := strings.Split(row, "\n")
		if len(lines) != 2 {
			t.Fatalf("mode=%s row=%q", mode, row)
		}
		// The gutter marks both lines; ASCII marks the first line only.
		markers := []string{"▌", "▌"}
		if mode == "ascii" {
			markers = []string{">", " "}
		}
		for index, line := range lines {
			if !strings.HasPrefix(sanitizeComponentRender(line), markers[index]) {
				t.Fatalf("mode=%s line %d missing marker in %q", mode, index, line)
			}
		}
	}
}

func TestVisibleTaskBlockRangeKeepsWholeRowsTogether(t *testing.T) {
	blocks := []TaskBlock{
		{Lines: []string{"heading"}, Height: 1},
		{UUID: "one", Lines: []string{"one", "meta"}, Height: 2},
		{Lines: []string{"", ""}, Height: 2},
		{UUID: "two", Lines: []string{"two", "meta"}, Height: 2},
	}
	start, end := VisibleTaskBlockRange(blocks, "two", 2)
	if start != 3 || end != 4 {
		t.Fatalf("range=%d,%d", start, end)
	}
	start, end = VisibleTaskBlockRange(blocks, "two", 4)
	if start >= 3 || end <= 3 {
		t.Fatalf("larger range=%d,%d", start, end)
	}
}

func TestReadableLayoutBoundariesAndGeometry(t *testing.T) {
	for _, tc := range []struct {
		width int
		mode  LayoutMode
		dense RowDensity
	}{
		{27, LayoutMinimum, ""}, {28, LayoutTabs, DensityCompact}, {49, LayoutTabs, DensityCompact},
		{50, LayoutTabs, DensityComfortable}, {103, LayoutTabs, DensityComfortable}, {104, LayoutWide, DensityComfortable},
	} {
		layout := ChooseLayout(tc.width, 18)
		if layout.Mode != tc.mode || layout.RowDensity != tc.dense {
			t.Fatalf("width=%d layout=%#v", tc.width, layout)
		}
		if layout.Usable && layout.MainWidth < 1 {
			t.Fatalf("width=%d layout has no main width: %#v", tc.width, layout)
		}
	}
	wide := ChooseLayout(120, 30)
	if !wide.ShowSidebar || wide.MainLeft != SidebarWidth+1+ContentGutter || wide.MainWidth != 91 {
		t.Fatalf("wide=%#v", wide)
	}
	for _, height := range []int{8, 12, 18, 30} {
		for _, search := range []bool{false, true} {
			geometry := ChooseLayout(79, height).Geometry(search)
			if geometry.BodyHeight < 1 || geometry.FooterTop+geometry.FooterHeight > height {
				t.Fatalf("height=%d search=%v geometry=%#v", height, search, geometry)
			}
		}
	}
}

func TestComponentStatesStayWithinTerminalBounds(t *testing.T) {
	states := []string{"task-comfortable", "task-compact", "details", "help", "quick-capture", "edit", "confirm", "quit", "settings-list", "settings-editor"}
	for _, size := range [][2]int{{120, 30}, {79, 24}, {49, 18}, {28, 8}} {
		for _, state := range states {
			view := fixtureComponentView(size[0], size[1], state)
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatalf("state=%s size=%v lines=%d", state, size, len(strings.Split(view, "\n")))
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatalf("state=%s size=%v width=%d line=%q", state, size, lipgloss.Width(line), line)
				}
			}
		}
	}
}

func TestDetailsAndHelpRetainScrollableContent(t *testing.T) {
	styles := NewStyles(ResolveTheme("dark", true))
	details := NewDetails(styles)
	details.SetSize(28, 8)
	details.OpenTask(domain.Task{
		UUID: "fixture", Description: strings.Repeat("long description ", 8), Status: "pending",
		RawFields: map[string]json.RawMessage{"custom": json.RawMessage(`"last field"`)},
	})
	for i := 0; i < 100; i++ {
		details.Update(keyForReadability("down"))
	}
	if view := details.View(); !strings.Contains(view, "custom") || !strings.Contains(view, "last") || !strings.Contains(view, "field") {
		t.Fatalf("details did not expose trailing field: %q", details.View())
	}

	help := NewHelp(styles)
	help.SetSize(30, 8)
	help.OpenHelp()
	for i := 0; i < 100; i++ {
		help.Update(keyForReadability("down"))
	}
	if !strings.Contains(help.View(), "scroll the") || !strings.Contains(help.View(), "list") {
		t.Fatalf("help did not expose trailing binding: %q", help.View())
	}
}

func keyForReadability(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}
