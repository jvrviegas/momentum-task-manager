package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// WidePlusBreakpoint adds a read-only details pane for the selected task:
	// past ~115 columns a single task line gets too long to scan.
	WidePlusBreakpoint = 140
	// WideBreakpoint is the first width at which a sidebar leaves the main
	// content with a comfortable reading width. Keep this value centralized:
	// changing it in one place must not change mouse or row geometry.
	WideBreakpoint = 104
	// NarrowBreakpoint is the first width at which comfortable two-line task
	// blocks fit. Widths below it use single-line rows.
	NarrowBreakpoint = 50
	// MinimalTabsBreakpoint is where the tab row keeps only the active view
	// and shows the others as position dots.
	MinimalTabsBreakpoint = 40
	// Minimum dimensions keep the warning itself usable.
	MinimumWidth  = 28
	MinimumHeight = 8

	// SidebarWidth is the wide rail: 23 cells and a blank column, then the
	// │ divider at column 24.
	SidebarWidth = 24
	// ContentGutter separates the divider from the main column, which starts
	// at column 27 and keeps a 2-column right margin.
	ContentGutter = 2
	MainLeftWide  = SidebarWidth + 1 + ContentGutter
	RightMargin   = 2
	// DetailsPaneWidth is the Wide+ pane, including its │ divider.
	DetailsPaneWidth = 44
)

// RowDensity identifies the two supported task-list presentations.
type RowDensity string

const (
	DensityComfortable RowDensity = "comfortable"
	DensityCompact     RowDensity = "compact"
)

// LayoutMode identifies the responsive composition.
type LayoutMode string

const (
	LayoutMinimum LayoutMode = "minimum"
	LayoutTabs    LayoutMode = "tabs"
	LayoutWide    LayoutMode = "wide"
)

// Layout contains centralized responsive decisions. MainLeft and MainWidth
// are the main column's absolute position, shared by rendering and mouse hit
// testing.
type Layout struct {
	Mode            LayoutMode
	Width           int
	Height          int
	SidebarWidth    int
	MainLeft        int
	MainWidth       int
	PaneLeft        int
	PaneWidth       int
	ShowSidebar     bool
	ShowTabs        bool
	ShowPane        bool
	Narrow          bool
	ShowRowMetadata bool
	RowDensity      RowDensity
	Usable          bool
}

// ShellGeometry is the shared vertical contract for rendering and mouse hit
// testing. Rows are absolute terminal rows; HintsTop is -1 when the footer is
// only the status bar.
type ShellGeometry struct {
	TerminalHeight int
	HeaderTop      int
	HeaderHeight   int
	NavTop         int
	SearchTop      int
	SearchHeight   int
	BodyTop        int
	BodyHeight     int
	HintsTop       int
	StatusTop      int
	FooterTop      int
	FooterHeight   int
}

// Geometry derives the shell rows. Wide and compact keep a title/tab row and
// its rule, a search-or-gap row, the body, a hints row and the status bar;
// wide also keeps a blank row where the sidebar divider ends. Narrow drops
// the hints row and the gap.
func (l Layout) Geometry(searchOpen bool) ShellGeometry {
	if l.Height <= 0 {
		return ShellGeometry{}
	}
	g := ShellGeometry{TerminalHeight: l.Height, HeaderHeight: 2, StatusTop: l.Height - 1}
	if searchOpen {
		g.SearchTop, g.SearchHeight = 2, 1
	}
	switch {
	case l.Narrow:
		g.HintsTop = -1
		g.FooterTop, g.FooterHeight = l.Height-1, 1
		g.BodyTop = 2 + g.SearchHeight
		g.BodyHeight = l.Height - 1 - g.BodyTop
	case l.ShowSidebar:
		g.HintsTop = l.Height - 2
		g.FooterTop, g.FooterHeight = l.Height-2, 2
		g.BodyTop = 3
		g.BodyHeight = l.Height - 6
	default:
		g.HintsTop = l.Height - 2
		g.FooterTop, g.FooterHeight = l.Height-2, 2
		g.BodyTop = 3
		g.BodyHeight = l.Height - 5
	}
	if g.BodyHeight < 1 {
		g.BodyHeight = 1
	}
	return g
}

// ChooseLayout maps terminal dimensions to a width-safe composition.
func ChooseLayout(width, height int) Layout {
	layout := Layout{
		Width:  width,
		Height: height,
		Usable: width >= MinimumWidth && height >= MinimumHeight,
	}
	if !layout.Usable {
		layout.Mode = LayoutMinimum
		return layout
	}

	layout.ShowRowMetadata = width >= NarrowBreakpoint
	if layout.ShowRowMetadata {
		layout.RowDensity = DensityComfortable
	} else {
		layout.RowDensity = DensityCompact
	}

	switch {
	case width >= WideBreakpoint:
		layout.Mode = LayoutWide
		layout.ShowSidebar = true
		layout.SidebarWidth = SidebarWidth
		layout.MainLeft = MainLeftWide
		layout.MainWidth = width - MainLeftWide - RightMargin
		if width >= WidePlusBreakpoint {
			layout.ShowPane = true
			layout.PaneWidth = DetailsPaneWidth
			layout.PaneLeft = width - DetailsPaneWidth
			layout.MainWidth -= DetailsPaneWidth
		}
	case width >= NarrowBreakpoint:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.MainLeft = 2
		layout.MainWidth = width - 4
	default:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.Narrow = true
		layout.MainLeft = 1
		layout.MainWidth = width - 2
	}
	if layout.MainWidth < 1 {
		layout.MainWidth = 1
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
	return ansi.Truncate(value, width, "…")
}

// PadRight adds display-width spaces, never byte-count spaces.
func PadRight(value string, width int) string {
	remaining := width - lipgloss.Width(value)
	if remaining <= 0 {
		return value
	}
	return value + spaces(remaining)
}

// WrapText wraps plain or ANSI-free content to display width. Long words are
// split by rune so a URL, UUID, or raw Taskwarrior value cannot make a modal
// wider than its terminal. Empty input returns one empty line for predictable
// field rendering.
func WrapText(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	paragraphs := strings.Split(value, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				if lipgloss.Width(word) <= width {
					line = word
					continue
				}
				chunks := splitDisplayWidth(word, width)
				lines = append(lines, chunks[:len(chunks)-1]...)
				line = chunks[len(chunks)-1]
				continue
			}
			candidate := line + " " + word
			if lipgloss.Width(candidate) <= width {
				line = candidate
				continue
			}
			lines = append(lines, line)
			if lipgloss.Width(word) <= width {
				line = word
				continue
			}
			chunks := splitDisplayWidth(word, width)
			lines = append(lines, chunks[:len(chunks)-1]...)
			line = chunks[len(chunks)-1]
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func splitDisplayWidth(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	chunks := make([]string, 0, (utf8.RuneCountInString(value)/width)+1)
	chunk := ""
	for _, r := range value {
		candidate := chunk + string(r)
		if lipgloss.Width(candidate) > width {
			if chunk == "" {
				// A single wide rune cannot fit a one-cell line. Keep the
				// width contract with the shared ellipsis rather than letting
				// CJK/emoji glyphs overflow a modal.
				chunks = append(chunks, Truncate(string(r), width))
				continue
			}
			chunks = append(chunks, chunk)
			chunk = string(r)
			continue
		}
		// A zero-width combining rune belongs to the current chunk.
		chunk = candidate
	}
	if chunk == "" {
		chunks = append(chunks, "")
	} else {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func spaces(count int) string {
	if count <= 0 {
		return ""
	}
	result := make([]byte, count)
	for index := range result {
		result[index] = ' '
	}
	return string(result)
}
