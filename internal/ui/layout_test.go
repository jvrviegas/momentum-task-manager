package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestChooseLayoutBreakpoints(t *testing.T) {
	wide := ChooseLayout(120, 30)
	if wide.Mode != LayoutWide || !wide.ShowSidebar || wide.ShowTabs || !wide.ShowRowMetadata || wide.ContentWidth != 95 {
		t.Fatalf("wide=%#v", wide)
	}
	tabs := ChooseLayout(79, 30)
	if tabs.Mode != LayoutTabs || tabs.ShowSidebar || !tabs.ShowTabs || !tabs.ShowRowMetadata {
		t.Fatalf("tabs=%#v", tabs)
	}
	narrow := ChooseLayout(49, 30)
	if narrow.Mode != LayoutTabs || !narrow.ShowTabs || narrow.ShowRowMetadata {
		t.Fatalf("narrow=%#v", narrow)
	}
}

func TestChooseLayoutMinimumSize(t *testing.T) {
	for _, size := range [][2]int{{27, 20}, {40, 7}, {0, 0}} {
		layout := ChooseLayout(size[0], size[1])
		if layout.Usable || layout.Mode != LayoutMinimum {
			t.Errorf("size=%v layout=%#v", size, layout)
		}
	}
	if !ChooseLayout(MinimumWidth, MinimumHeight).Usable {
		t.Fatal("minimum dimensions should be usable")
	}
}

func TestTruncateUsesDisplayWidthAndEllipsis(t *testing.T) {
	if got := Truncate("abcdef", 4); got != "abc…" {
		t.Fatalf("got %q", got)
	}
	if got := Truncate("東京タスク", 5); got == "" || lipgloss.Width(got) > 5 {
		t.Fatalf("unicode result=%q width=%d", got, lipgloss.Width(got))
	}
	if got := Truncate("long", 1); got != "…" {
		t.Fatalf("one-column result=%q", got)
	}
	if got := Truncate("ok", 10); got != "ok" {
		t.Fatalf("untruncated result=%q", got)
	}
}

func TestTruncateHandlesZeroAndNegativeWidths(t *testing.T) {
	if Truncate("value", 0) != "" || Truncate("value", -1) != "" {
		t.Fatal("invalid widths should be empty")
	}
}

func TestPadRightUsesDisplayWidth(t *testing.T) {
	got := PadRight("東京", 6)
	if got != "東京  " {
		t.Fatalf("got %q", got)
	}
	if PadRight("already", 2) != "already" {
		t.Fatal("padding should not truncate")
	}
}

func TestLayoutContentWidthNeverDropsBelowOne(t *testing.T) {
	layout := ChooseLayout(MinimumWidth, MinimumHeight)
	if layout.ContentWidth < 1 {
		t.Fatalf("layout=%#v", layout)
	}
}
