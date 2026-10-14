package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestChooseLayoutBreakpoints(t *testing.T) {
	wide := ChooseLayout(120, 30)
	if wide.Mode != LayoutWide || !wide.ShowSidebar || wide.ShowTabs || !wide.ShowRowMetadata || wide.MainLeft != 27 || wide.MainWidth != 91 || wide.ShowPane {
		t.Fatalf("wide=%#v", wide)
	}
	tabs := ChooseLayout(79, 30)
	if tabs.Mode != LayoutTabs || tabs.ShowSidebar || !tabs.ShowTabs || !tabs.ShowRowMetadata {
		t.Fatalf("tabs=%#v", tabs)
	}
	if tabs.MainLeft != 2 || tabs.MainWidth != 75 {
		t.Fatalf("compact main column=%#v", tabs)
	}
	narrow := ChooseLayout(49, 30)
	if narrow.Mode != LayoutTabs || !narrow.ShowTabs || narrow.ShowRowMetadata || !narrow.Narrow || narrow.MainLeft != 1 || narrow.MainWidth != 47 {
		t.Fatalf("narrow=%#v", narrow)
	}
	plus := ChooseLayout(150, 30)
	if !plus.ShowPane || plus.PaneLeft != 106 || plus.PaneWidth != 44 || plus.MainWidth != 77 {
		t.Fatalf("wide+=%#v", plus)
	}
}

func TestShellGeometryRowsPerTier(t *testing.T) {
	for _, tc := range []struct {
		width, height                      int
		search                             bool
		bodyTop, bodyHeight, hints, status int
	}{
		{120, 30, false, 3, 24, 28, 29},
		{120, 30, true, 3, 24, 28, 29},
		{79, 24, false, 3, 19, 22, 23},
		{49, 18, false, 2, 15, -1, 17},
		{49, 18, true, 3, 14, -1, 17},
		{28, 8, false, 2, 5, -1, 7},
	} {
		g := ChooseLayout(tc.width, tc.height).Geometry(tc.search)
		if g.BodyTop != tc.bodyTop || g.BodyHeight != tc.bodyHeight || g.HintsTop != tc.hints || g.StatusTop != tc.status {
			t.Errorf("size=%dx%d search=%v geometry=%#v", tc.width, tc.height, tc.search, g)
		}
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
	if layout.MainWidth < 1 {
		t.Fatalf("layout=%#v", layout)
	}
}
