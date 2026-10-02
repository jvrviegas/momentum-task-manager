package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

func TestColorStrippedShellRetainsActiveAndErrorStatus(t *testing.T) {
	model := readyCompositionModel(79, 24)
	plain := ansi.Strip(model.View().Content)
	lines := strings.Split(plain, "\n")
	today := strings.Index(lines[0], "Today")
	if today < 0 || len(lines) < 2 || string([]rune(lines[1])[lipgloss.Width(lines[0][:today])]) != "━" {
		t.Fatalf("active view lost its non-color cue: %q", plain)
	}
	if !strings.Contains(plain, "Sync ready") {
		t.Fatalf("sync status lost from color-stripped footer: %q", plain)
	}

	model.Tasks = nil
	model.Views = domain.Views{}
	model.Err = errors.New("fixture Taskwarrior unavailable")
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "! fixture Taskwarrior unavailable") {
		t.Fatalf("error status lost from color-stripped footer: %q", plain)
	}
}

func TestMouseSelectsBothLinesOfComfortableTaskBlock(t *testing.T) {
	model := readyCompositionModel(79, 24)
	model.ActiveView = ViewToday
	model.Selected[ViewToday] = "overdue"
	model.Selections[ViewToday] = 0
	layout := ui.ChooseLayout(model.Width, model.Height)
	geometry := layout.Geometry(false)
	blocks := model.taskBlocks(layout.MainWidth)
	start, end := visibleRenderedBlockRange(blocks, model.Selected[ViewToday], geometry.BodyHeight)
	// Derive the absolute row without relying on a fixed header offset.
	line := -1
	cursor := geometry.BodyTop
	for _, block := range blocks[start:end] {
		if block.uuid == "due" && len(block.lines) > 1 {
			line = cursor + 1
			break
		}
		cursor += len(block.lines)
	}
	if line < 0 {
		t.Fatal("due task metadata line was not rendered")
	}
	model.Update(tea.MouseClickMsg{X: 30, Y: line, Button: tea.MouseLeft})
	if model.Selected[ViewToday] != "due" || model.Selections[ViewToday] != 1 {
		t.Fatalf("selected=%q index=%d line=%d", model.Selected[ViewToday], model.Selections[ViewToday], line)
	}
}
