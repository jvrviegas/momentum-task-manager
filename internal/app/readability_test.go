package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/ui"
)

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
