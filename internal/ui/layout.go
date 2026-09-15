package ui

import "charm.land/lipgloss/v2"

const (
	// WideBreakpoint is the first width at which the sidebar is useful.
	WideBreakpoint = 80
	// NarrowBreakpoint hides nonessential task-row metadata.
	NarrowBreakpoint = 50
	// Minimum dimensions keep the warning itself usable.
	MinimumWidth  = 28
	MinimumHeight = 8
	SidebarWidth  = 24
)

// LayoutMode identifies the responsive composition.
type LayoutMode string

const (
	LayoutMinimum LayoutMode = "minimum"
	LayoutTabs    LayoutMode = "tabs"
	LayoutWide    LayoutMode = "wide"
)

// Layout contains centralized responsive decisions.
type Layout struct {
	Mode            LayoutMode
	Width           int
	Height          int
	SidebarWidth    int
	ContentWidth    int
	ShowSidebar     bool
	ShowTabs        bool
	ShowRowMetadata bool
	Usable          bool
}

// ChooseLayout maps terminal dimensions to a width-safe composition.
func ChooseLayout(width, height int) Layout {
	layout := Layout{Width: width, Height: height, Usable: width >= MinimumWidth && height >= MinimumHeight}
	if !layout.Usable {
		layout.Mode = LayoutMinimum
		return layout
	}
	layout.ShowRowMetadata = width >= NarrowBreakpoint
	switch {
	case width >= WideBreakpoint:
		layout.Mode = LayoutWide
		layout.ShowSidebar = true
		layout.SidebarWidth = SidebarWidth
		layout.ContentWidth = width - SidebarWidth - 1
	case width >= NarrowBreakpoint:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.ContentWidth = width
	default:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.ContentWidth = width
	}
	if layout.ContentWidth < 1 {
		layout.ContentWidth = 1
	}
	return layout
}

// Truncate keeps a string within a terminal display width without splitting a
// UTF-8 rune. It does not count ANSI escape sequences because Lip Gloss does
// not count them either.
func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	available := width - lipgloss.Width("…")
	if available <= 0 {
		return "…"
	}
	result := make([]rune, 0, len([]rune(value)))
	used := 0
	for _, char := range []rune(value) {
		charWidth := lipgloss.Width(string(char))
		if used+charWidth > available {
			break
		}
		result = append(result, char)
		used += charWidth
	}
	return string(result) + "…"
}

// PadRight adds display-width spaces, never byte-count spaces.
func PadRight(value string, width int) string {
	remaining := width - lipgloss.Width(value)
	if remaining <= 0 {
		return value
	}
	return value + spaces(remaining)
}

func spaces(count int) string {
	result := make([]byte, count)
	for index := range result {
		result[index] = ' '
	}
	return string(result)
}
